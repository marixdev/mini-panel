package main

import (
	"errors"
	"flag"
	"fmt"
	"sort"
	"strings"

	"mpn/internal/dbadmin"
	"mpn/internal/distro"
	"mpn/internal/firewall"
	"mpn/internal/installer"
	"mpn/internal/store"
)

// Remote access to the databases of sites. The server listens on 127.0.0.1 only until a database is opened to an address; then it
// listens on every address, and the firewall (when it is on) lets port 3306 in from the addresses that were given and from nowhere
// else. When the last database is closed again, the server goes back to 127.0.0.1.

const (
	dbPort        = 3306
	dbFirewallTag = "mpn: remote database access"
	// The server reads the files of the folder in byte order and the last value wins. "_" sorts after ".", so this file comes after
	// 99-mpn.cnf; "99-mpn-remote.cnf" would come before it and its bind-address would be overridden (seen on MariaDB 11).
	dbRemoteConfFile  = "99-mpn_remote.cnf"
	settingDBRemoteOn = "db_remote_listen"
)

// dbRemoteConf is the file that makes the server listen on every address. It sorts after 99-mpn.cnf, so its bind-address wins.
func dbRemoteConf(engine string) string {
	return strings.TrimSuffix(distro.DBConfFile(engine), "99-mpn.cnf") + dbRemoteConfFile
}

func dbEngine() (string, error) {
	switch unitOf("database") {
	case "mariadb.service":
		return "mariadb", nil
	case "":
		return "", errors.New("there is no database server on this machine")
	}
	return "mysql", nil
}

// dbRemoteInfo is the answer of "mpn db remote -name X -json" with no -hosts: where the database can be reached from.
type dbRemoteInfo struct {
	Name       string   `json:"name"`
	Hosts      []string `json:"hosts"`
	Listening  bool     `json:"listening"`   // the server listens on every address
	FirewallOn bool     `json:"firewall_on"` // without it, the port is open to everyone and only the users' hosts decide
	Port       int      `json:"port"`
}

func dbRemote(args []string) {
	fs := flag.NewFlagSet("db remote", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	name := fs.String("name", "", "database (and user) name")
	hosts := fs.String("hosts", "", "addresses the user may connect from: IPs, IPv4 networks such as 203.0.113.0/24, or % for anywhere; "+
		"\"none\" closes remote access")
	asJSON := fs.Bool("json", false, "print JSON")
	apply := fs.Bool("apply", false, "change the database server and the firewall (Linux, root)")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	d, err := st.GetDatabase(*name)
	fail(err)
	on, _ := st.FirewallOn()
	if *hosts == "" { // only show
		requireLinuxRoot()
		hs, err := dbAdmin().Hosts(d.Name)
		fail(err)
		v, _ := st.Setting(settingDBRemoteOn, "off")
		info := dbRemoteInfo{Name: d.Name, Hosts: append([]string{}, hs...), Listening: v == "on", FirewallOn: on, Port: dbPort}
		if *asJSON {
			printJSON(info)
			return
		}
		fmt.Printf("%s: %s\n", d.Name, map[bool]string{true: strings.Join(hs, ", "), false: "this machine only"}[len(hs) > 0])
		return
	}
	want := []string{}
	if strings.TrimSpace(*hosts) != "none" {
		want, err = dbadmin.ParseHosts(*hosts)
		if err != nil {
			fail(fmt.Errorf("db-hosts: %w", err))
		}
	}
	if !*apply {
		fmt.Printf("would let %q be reached from: %v (port %d)\n", d.Name, want, dbPort)
		return
	}
	requireLinuxRoot()
	engine, err := dbEngine()
	fail(err)
	fail(dbAdmin().SetHosts(d.Name, want, engine == "mysql"))
	fail(dbRemoteSync(st, engine))
	if len(want) == 0 {
		fmt.Printf("%q can be reached from this machine only\n", d.Name)
		return
	}
	fmt.Printf("%q can be reached from %s on port %d, with the same user and password\n", d.Name, strings.Join(want, ", "), dbPort)
	if !on {
		fmt.Println("warning: the firewall is off, so port 3306 is open to every address; only the user's allowed addresses keep others out. Turn the firewall on in Security.")
	}
}

// dbRemoteSync makes the server and the firewall match the users that exist: the addresses of every database's remote users are the
// firewall rules for port 3306, and the server listens on every address while there is at least one.
func dbRemoteSync(st *store.Store, engine string) error {
	adm := dbAdmin()
	list, err := st.ListDatabases("")
	if err != nil {
		return err
	}
	set := map[string]bool{}
	for _, d := range list {
		hs, err := adm.Hosts(d.Name)
		if err != nil {
			return err
		}
		for _, h := range hs {
			set[h] = true
		}
	}
	sources := make([]string, 0, len(set))
	for h := range set {
		sources = append(sources, h)
	}
	sort.Strings(sources)

	// The rules for port 3306 are replaced by the ones these addresses need. "%" is every address: one rule with no source.
	rules, err := st.FirewallRules()
	if err != nil {
		return err
	}
	for _, r := range rules {
		if r.Comment == dbFirewallTag {
			if err := st.DeleteFirewallRule(r.ID); err != nil {
				return err
			}
		}
	}
	for _, h := range sources {
		r := firewall.Rule{Action: firewall.Allow, Proto: "tcp", Port: fmt.Sprint(dbPort), Comment: dbFirewallTag}
		if h != dbadmin.AnyHost {
			r.Source = h
		}
		if r, err = firewall.ValidRule(r); err != nil {
			return err
		}
		if _, err := st.AddFirewallRule(r); err != nil && !errors.Is(err, store.ErrExists) {
			return err
		}
	}
	if err := mailFirewallReload(st); err != nil {
		return err
	}

	listen := len(sources) > 0
	was, _ := st.Setting(settingDBRemoteOn, "off")
	if listen == (was == "on") {
		return nil
	}
	conf, svc := dbRemoteConf(engine), distro.DBService(engine)
	var act installer.Action
	if listen {
		body := "# Made by mpn: some databases may be reached from other machines (mpn db remote).\n[mysqld]\nbind-address = 0.0.0.0\n"
		act = installer.WriteFile(conf, body, 0o644)
	} else {
		act = installer.Run("rm", "-f", conf)
	}
	if err := runPlanQuiet(installer.Plan{{Name: "database server address", Actions: []installer.Action{act, installer.Run("systemctl", "restart", svc)}}}); err != nil {
		return err
	}
	return st.SetSetting(settingDBRemoteOn, map[bool]string{true: "on", false: "off"}[listen])
}
