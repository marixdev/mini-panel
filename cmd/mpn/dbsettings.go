package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"mpn/internal/distro"
	"mpn/internal/tune"
)

// The settings of the database server that a person may change from the panel. They live in a file of their own after mpn's tuning,
// so they win over it and survive a reinstall of the tuning. A change restarts the server; if it does not come back, the old file is
// put back and the server started again, so a value the server refuses never leaves the websites without a database.

// After 99-mpn.cnf in byte order ("_" sorts after "."), so these values win over mpn's tuning. See dbRemoteConfFile.
const dbUserConfFile = "99-mpn_user.cnf"

func dbUserConf(engine string) string {
	return strings.TrimSuffix(distro.DBConfFile(engine), "99-mpn.cnf") + dbUserConfFile
}

// dbSettingsInfo is the answer of "mpn db settings -json": what can be set, and what is set now.
type dbSettingsInfo struct {
	Engine   string            `json:"engine"`
	Settings []tune.DBSetting  `json:"settings"`
	Values   map[string]string `json:"values"`  // what the panel set; a key not here is the server's own value
	Current  map[string]string `json:"current"` // what the running server says, for every key the panel offers
}

func dbSettings(args []string) {
	fs := flag.NewFlagSet("db settings", flag.ExitOnError)
	stdin := fs.Bool("stdin", false, "read the new values as a JSON object on standard input; an empty value goes back to the server's own")
	apply := fs.Bool("apply", false, "write the settings and restart the database server (Linux, root)")
	_ = fs.Parse(args)
	requireLinuxRoot()
	engine, err := dbEngine()
	fail(err)
	path := dbUserConf(engine)
	old, _ := os.ReadFile(path)
	if !*stdin {
		info := dbSettingsInfo{Engine: engine, Settings: tune.DBSettings, Values: tune.ParseUserDBConf(string(old)), Current: dbCurrent()}
		printJSON(info)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 16<<10))
	fail(err)
	var in map[string]string
	if err := json.Unmarshal(raw, &in); err != nil {
		fail(fmt.Errorf("db-settings: the input is not a JSON object of strings: %w", err))
	}
	values, err := tune.CheckDBValues(in)
	if err != nil {
		fail(fmt.Errorf("db-settings: %w", err))
	}
	text := tune.UserDBConf(values)
	if !*apply {
		fmt.Print(text)
		return
	}
	if values["slow_query_log"] == "ON" {
		text = strings.Replace(text, "slow_query_log = ON\n", "slow_query_log = ON\nslow_query_log_file = /var/log/mysql/slow.log\n", 1)
		_ = os.MkdirAll("/var/log/mysql", 0o750)
		_ = exec.Command("chown", "mysql:mysql", "/var/log/mysql").Run()
	}
	svc := distro.DBService(engine)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		fail(err)
	}
	if err := restartAndWait(svc); err != nil {
		// Put the old file back (or none) and start again: the sites must keep their database.
		if len(old) > 0 {
			_ = os.WriteFile(path, old, 0o644)
		} else {
			_ = os.Remove(path)
		}
		back := restartAndWait(svc)
		msg := "db-settings-refused: the database server did not start with these settings, so the previous ones were put back"
		if back != nil {
			msg += "; it did not start with those either: " + back.Error()
		}
		fail(errors.New(msg + ". " + lastLog(svc)))
	}
	fmt.Printf("database settings saved and the server restarted (%d set by the panel)\n", len(values))
}

// restartAndWait restarts the server and waits until it answers, up to a minute.
func restartAndWait(svc string) error {
	if out, err := exec.Command("systemctl", "restart", svc).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	client, err := exec.LookPath("mariadb-admin")
	if err != nil {
		client, err = exec.LookPath("mysqladmin")
	}
	if err != nil {
		return nil // nothing to ask; systemd said it started
	}
	for i := 0; i < 30; i++ {
		if exec.Command(client, "ping", "--silent").Run() == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return errors.New("the server does not answer")
}

func lastLog(svc string) string {
	out, _ := exec.Command("journalctl", "-u", svc, "-n", "6", "--no-pager", "-o", "cat").Output()
	return strings.TrimSpace(string(out))
}

// dbCurrent asks the running server for the values of the keys the panel offers.
func dbCurrent() map[string]string {
	out := map[string]string{}
	keys := make([]string, 0, len(tune.DBSettings))
	for _, s := range tune.DBSettings {
		keys = append(keys, "'"+s.Key+"'")
	}
	text, err := dbAdmin().R.Run("SHOW GLOBAL VARIABLES WHERE Variable_name IN (" + strings.Join(keys, ",") + ");\n")
	if err != nil {
		return out
	}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if k, v, ok := strings.Cut(line, "\t"); ok {
			out[k] = v
		}
	}
	return out
}
