package monitor

import (
	"os/exec"
	"strings"
	"time"

	"mpn/internal/distro"
)

// Service is a program the websites depend on, and whether it is running.
type Service struct {
	Unit   string `json:"unit"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
	State  string `json:"state"` // what systemd says: active, inactive, failed, activating...
}

// Watched are the services shown on the page: the unit name and what a person calls it. A unit that does not exist on this
// machine (no MariaDB, a PHP version that is not installed) is left out, not shown as stopped.
var Watched = []struct{ Unit, Label string }{
	{"nginx.service", "nginx"},
	{"mariadb.service", "MariaDB"},
	{"mysql.service", "MySQL"},
	{"mysqld.service", "MySQL"}, // the name of the MySQL unit on the RHEL family
	{"cron.service", "cron"},
	{"crond.service", "cron"}, // the name on the RHEL family
	{"ssh.service", "SSH"},
	{"sshd.service", "SSH"},
	{"mpn-root.service", "svc.unit.mpn-root"},
	{"mpn-web.service", "svc.unit.mpn-web"},
	{"mpn-firewall.service", "svc.unit.mpn-firewall"},
}

// A Label that starts with LabelKeyPrefix is a message key of the web interface, translated there; any other Label is a
// proper name (nginx, MariaDB) shown as it is.
const LabelKeyPrefix = "svc.unit."

// UnitState asks systemd about one unit and returns its real name and its state. The name is "" when no such unit exists. It is
// the real name, not the one asked for, because several names can lead to one unit: mysql.service is another name of
// mariadb.service, and sshd.service of ssh.service.
type UnitState func(unit string) (id, state string)

// SystemdState asks the real systemd.
func SystemdState(unit string) (string, string) {
	run := func(args ...string) string {
		out, _ := exec.Command("systemctl", args...).Output()
		return strings.TrimSpace(string(out))
	}
	done := make(chan [2]string, 1)
	go func() {
		id, loaded := "", false
		for _, line := range strings.Split(run("show", "-p", "Id", "-p", "LoadState", unit), "\n") {
			switch k, v, _ := strings.Cut(line, "="); k {
			case "Id":
				id = v
			case "LoadState":
				loaded = v == "loaded"
			}
		}
		if !loaded {
			id = ""
		}
		done <- [2]string{id, run("is-active", unit)}
	}()
	select {
	case r := <-done:
		return r[0], r[1]
	case <-time.After(3 * time.Second):
		return "", ""
	}
}

// Services returns the state of every watched unit that exists, and of the PHP-FPM versions given. A unit that is reached by two
// names is shown once, under the first.
func Services(state UnitState, phpVersions []string) []Service {
	type item struct{ unit, label string }
	var items []item
	for _, w := range Watched {
		items = append(items, item{w.Unit, w.Label})
	}
	for _, v := range phpVersions {
		items = append(items, item{distro.PHPService(v) + ".service", "PHP-FPM " + v})
	}
	var out []Service
	seenID, seenLabel := map[string]bool{}, map[string]bool{}
	for _, it := range items {
		id, st := state(it.unit)
		if id == "" || seenID[id] || seenLabel[it.label] {
			continue
		}
		seenID[id], seenLabel[it.label] = true, true
		out = append(out, Service{Unit: id, Label: it.label, Active: st == "active", State: st})
	}
	return out
}
