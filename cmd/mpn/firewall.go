package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"mpn/internal/firewall"
	"mpn/internal/installer"
	"mpn/internal/store"
)

const (
	nftBin       = "/usr/sbin/nft"
	fwFile       = "/etc/mpn/firewall.nft"
	fwOffFile    = "/etc/mpn/firewall-off.nft"
	fwUnitFile   = "/etc/systemd/system/mpn-firewall.service"
	fwUnitName   = "mpn-firewall.service"
	panelNginx   = "/etc/nginx/conf.d/01-mpn-panel.conf"
	fwUnitSource = "[Unit]\nDescription=Mini Panel firewall\nBefore=network-pre.target\nWants=network-pre.target\nAfter=local-fs.target\n\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=/usr/local/bin/mpn firewall load\nExecStop=" + nftBin + " -f " + fwOffFile + "\n\n[Install]\nWantedBy=multi-user.target\n"
)

func firewallUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn firewall <status|enable|disable|add|remove|load> [flags]   (add -h for the flags)")
	os.Exit(2)
}

// out runs a command and returns what it printed; an error or a missing program gives "".
func out(timeout time.Duration, name string, args ...string) string {
	o, _ := sh(timeout, name, args...)
	return o
}

var (
	sshPortRe   = regexp.MustCompile(`(?m)^port (\d+)$`)
	listenSSLRe = regexp.MustCompile(`listen\s+(\d+)\s+ssl`)
)

// lockedPorts finds the ports that must stay open: those of SSH (from the configuration of sshd, which is right even before
// sshd runs, and from the sockets it listens on) and those of the panel and of its database browser (from the nginx files mpn
// wrote). When a kind cannot be found its usual port is used, so the way in is never empty.
func lockedPorts() firewall.Locked {
	var l firewall.Locked
	for _, m := range sshPortRe.FindAllStringSubmatch(out(5*time.Second, "sshd", "-T"), -1) {
		n, _ := strconv.Atoi(m[1])
		l.SSH = append(l.SSH, n)
	}
	for _, line := range strings.Split(out(5*time.Second, "ss", "-ltnpH"), "\n") {
		if !strings.Contains(line, `"sshd"`) {
			continue
		}
		f := strings.Fields(line)
		if len(f) > 3 {
			if _, port, err := net.SplitHostPort(f[3]); err == nil {
				n, _ := strconv.Atoi(port)
				l.SSH = append(l.SSH, n)
			}
		}
	}
	if len(l.SSH) == 0 {
		l.SSH = []int{22}
	}
	if b, err := os.ReadFile(panelNginx); err == nil {
		for _, m := range listenSSLRe.FindAllStringSubmatch(string(b), -1) {
			n, _ := strconv.Atoi(m[1])
			l.Panel = append(l.Panel, n)
		}
	}
	if matches, _ := filepath.Glob("/etc/nginx/conf.d/*mpn*pma*.conf"); len(matches) > 0 {
		for _, f := range matches {
			if b, err := os.ReadFile(f); err == nil {
				for _, m := range listenSSLRe.FindAllStringSubmatch(string(b), -1) {
					n, _ := strconv.Atoi(m[1])
					l.Panel = append(l.Panel, n)
				}
			}
		}
	}
	if len(l.Panel) == 0 {
		l.Panel = []int{2610}
	}
	l.SSH, l.Panel = uniqSorted(l.SSH), uniqSorted(l.Panel)
	return l
}

// uniqSorted sorts ports and drops repeats: sshd listens on IPv4 and IPv6, so the same port is found more than once.
func uniqSorted(ports []int) []int {
	sort.Ints(ports)
	return slices.Compact(ports)
}

// conflict names another firewall that is running; two of them together make rules nobody can read.
func conflict() string {
	// The service of ufw is "active" on a machine where ufw is installed but turned off, so the service says nothing; ufw itself does.
	if strings.Contains(out(5*time.Second, "ufw", "status"), "Status: active") {
		return "ufw"
	}
	if out(3*time.Second, "systemctl", "is-active", "firewalld") == "active" {
		return "firewalld"
	}
	return ""
}

// fwStatus is the answer of "mpn firewall status -json".
type fwStatus struct {
	On       bool            `json:"on"`       // a person turned it on
	Active   bool            `json:"active"`   // the table is loaded in the kernel right now
	Conflict string          `json:"conflict"` // another firewall that is running
	HasNft   bool            `json:"has_nft"`
	Locked   firewall.Locked `json:"locked"`
	Rules    []firewall.Rule `json:"rules"`
}

func currentStatus(st *store.Store) fwStatus {
	on, _ := st.FirewallOn()
	rules, _ := st.FirewallRules()
	s := fwStatus{On: on, Conflict: conflict(), Locked: lockedPorts(), Rules: rules}
	if _, err := os.Stat(nftBin); err == nil {
		s.HasNft = true
		_, err := exec.Command(nftBin, "list", "table", "inet", "mpn").Output()
		s.Active = err == nil
	}
	return s
}

// loadPlan writes the rules and loads them. nft -c checks the whole script first, so a rule it does not understand never gets
// as far as the running firewall, and nft -f loads it in one step.
func loadPlan(rules []firewall.Rule) installer.Plan {
	script := firewall.Render(lockedPorts(), rules)
	return installer.Plan{{Name: "firewall rules", Actions: []installer.Action{
		installer.Run("install", "-d", "-m", "0755", "-o", "root", "-g", "root", "/etc/mpn"),
		installer.WriteFile(fwFile, script, 0o600),
		installer.Run(nftBin, "-c", "-f", fwFile),
		installer.Run(nftBin, "-f", fwFile),
	}}}
}

func enablePlan(rules []firewall.Rule) installer.Plan {
	plan := installer.Plan{{Name: "nftables", Actions: []installer.Action{
		installer.Run("sh", "-c", "[ -x "+nftBin+" ] || DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 install -y nftables"),
		installer.WriteFile(fwOffFile, firewall.Unload, 0o600),
		installer.WriteFile(fwUnitFile, fwUnitSource, 0o644),
		installer.Run("systemctl", "daemon-reload"),
	}}}
	plan = append(plan, loadPlan(rules)...)
	// Enabled after the rules loaded: a unit that fails at boot is easier to see than a firewall that never started.
	return append(plan, installer.Step{Name: "start at boot", Actions: []installer.Action{installer.Run("systemctl", "enable", fwUnitName)}})
}

func disablePlan() installer.Plan {
	return installer.Plan{{Name: "remove the firewall", Actions: []installer.Action{
		// The unit stays "active" after it has loaded the rules (RemainAfterExit), so it is stopped as well as disabled: stopping
		// it runs the script that deletes the table, and the state it reports is then the truth. The table is also deleted
		// directly, for a machine where the unit was never made.
		installer.Run("sh", "-c", nftBin+" list table inet mpn >/dev/null 2>&1 && "+nftBin+" delete table inet mpn || true"),
		installer.Run("sh", "-c", "systemctl disable --now "+fwUnitName+" 2>/dev/null || true"),
	}}}
}

// runFirewall manages the firewall of the server. It filters what comes in, in a table of nftables that belongs to mpn; the
// ports of SSH and of the panel are always open. Rules are stored in the database and loaded from it, also at boot.
func runFirewall(args []string) {
	if len(args) == 0 {
		firewallUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("firewall "+sub, flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	asJSON := fs.Bool("json", false, "status: print JSON")
	action := fs.String("action", "", "add: allow or deny")
	proto := fs.String("proto", "tcp", "add: tcp, udp or both")
	port := fs.String("port", "", "add: a port or a range such as 8000-8100; empty for every port")
	source := fs.String("source", "", "add: an address or a network; empty for every address")
	comment := fs.String("comment", "", "add: a note for the person")
	client := fs.String("client", "", "add: the address the person works from, so a rule that blocks it is refused")
	id := fs.Int64("id", 0, "remove: the number of the rule")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])
	st := openStore(*db)
	defer st.Close()

	switch sub {
	case "status":
		s := currentStatus(st)
		if *asJSON {
			printJSON(s)
			return
		}
		fmt.Printf("on: %v  loaded: %v  other firewall: %q\nSSH ports %v, panel ports %v\n", s.On, s.Active, s.Conflict, s.Locked.SSH, s.Locked.Panel)
		for _, r := range s.Rules {
			fmt.Printf("%d\t%s\t%s\tport %q\tfrom %q\t%s\n", r.ID, r.Action, r.Proto, r.Port, r.Source, r.Comment)
		}
		return
	case "load":
		on, err := st.FirewallOn()
		fail(err)
		if !on {
			fmt.Println("the firewall is off; nothing to load")
			return
		}
		rules, err := st.FirewallRules()
		fail(err)
		fail(runPlanQuiet(loadPlan(rules)))
		fmt.Println("firewall loaded")
		return
	case "enable":
		if c := conflict(); c != "" {
			fail(fmt.Errorf("firewall-conflict: %s is running. Two firewalls together give rules nobody can read; turn %s off first (systemctl disable --now %s)", c, c, c))
		}
		rules, err := st.FirewallRules()
		fail(err)
		first := false
		if v, _ := st.Setting("firewall", ""); v == "" && len(rules) == 0 {
			first, rules = true, firewall.Defaults()
		}
		if !*apply {
			fmt.Printf("# checked only. Would allow SSH %v and the panel %v, then:\n%s", lockedPorts().SSH, lockedPorts().Panel, firewall.Render(lockedPorts(), rules))
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(enablePlan(rules)))
		if first {
			for _, r := range rules {
				_, err := st.AddFirewallRule(r)
				fail(err)
			}
		}
		fail(st.SetFirewallOn(true))
		fmt.Println("the firewall is on")
		return
	case "disable":
		if !*apply {
			fmt.Println("# checked only. Add -apply to turn the firewall off.")
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(disablePlan()))
		fail(st.SetFirewallOn(false))
		fmt.Println("the firewall is off; the rules are kept")
		return
	case "add", "remove":
	default:
		firewallUsage()
	}

	rules, err := st.FirewallRules()
	fail(err)
	locked := lockedPorts()
	var added firewall.Rule
	if sub == "add" {
		var verr error
		added, verr = firewall.ValidRule(firewall.Rule{Action: *action, Proto: *proto, Port: *port, Source: *source, Comment: *comment})
		fail(verr)
		if *client != "" && net.ParseIP(*client) == nil {
			fail(fmt.Errorf("-client %q is not an address", *client))
		}
		fail(firewall.CheckLockout(added, locked, *client))
		rules = append(rules, added)
	} else {
		kept := rules[:0:0]
		found := false
		for _, r := range rules {
			if r.ID == *id {
				found = true
				continue
			}
			kept = append(kept, r)
		}
		if !found {
			fail(fmt.Errorf("there is no firewall rule %d", *id))
		}
		rules = kept
	}
	on, _ := st.FirewallOn()
	if !*apply {
		fmt.Printf("# checked only. With the firewall on, this is what would be loaded:\n%s", firewall.Render(locked, rules))
		return
	}
	requireLinuxRoot()
	if on {
		fail(runPlanQuiet(loadPlan(rules)))
	}
	if sub == "add" {
		n, err := st.AddFirewallRule(added)
		fail(err)
		fmt.Printf("rule %d added%s\n", n, map[bool]string{true: "", false: " (the firewall is off, so it is not loaded yet)"}[on])
	} else {
		fail(st.DeleteFirewallRule(*id))
		fmt.Printf("rule %d removed\n", *id)
	}
}
