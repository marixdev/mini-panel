// Package services lists what the panel manages on a machine and what it can still install: the services that run as units (nginx, the
// database, PHP-FPM, the mail programs), the PHP and Node.js versions, and the database browser. The list is fixed here and follows what
// the installer of mpn really supports; a key that is not in it is refused, so the web side never names a unit of its own.
package services

import (
	"fmt"
	"slices"
	"strings"

	"mpn/internal/installer"
	"mpn/internal/monitor"
	"mpn/internal/site"
)

// The actions on a service. "reload" is offered only where the program can re-read its configuration without stopping.
const (
	Start   = "start"
	Stop    = "stop"
	Restart = "restart"
	Reload  = "reload"
)

// The groups of the page, in the order it draws them.
const (
	GroupService  = "service"
	GroupDatabase = "database"
	GroupPHP      = "php"
	GroupNode     = "node"
	GroupTool     = "tool"
)

// Def is a program that runs as a unit and is managed once it exists.
type Def struct {
	Key, Label string
	Group      string
	Units      []string // the first one that exists is the service; several names can lead to one program
	Actions    []string
	// Link is the page that sets the program up, for those the panel installs from a page of their own (mail, fail2ban). Absent says
	// the row is shown even when the program is not there, with a link to that page instead of a button.
	Link   string
	Absent bool
}

// Defs is every unit except PHP-FPM, which has one entry per version (see PHPKey). The panel itself, SSH and the firewall are not here:
// stopping them would lock the person out, and the Monitor page already shows them.
var Defs = []Def{
	{Key: "nginx", Label: "nginx", Group: GroupService, Units: []string{"nginx.service"}, Actions: []string{Start, Restart, Reload}},
	{Key: "database", Label: "MariaDB / MySQL", Group: GroupDatabase, Units: []string{"mariadb.service", "mysql.service"}, Actions: []string{Start, Stop, Restart}},
	{Key: "fail2ban", Label: "fail2ban", Group: GroupService, Units: []string{"fail2ban.service"}, Actions: []string{Start, Stop, Restart, Reload}, Link: "/security", Absent: true},
	{Key: "postfix", Label: "Postfix", Group: GroupService, Units: []string{"postfix.service"}, Actions: []string{Start, Stop, Restart, Reload}, Link: "/email", Absent: true},
	{Key: "dovecot", Label: "Dovecot", Group: GroupService, Units: []string{"dovecot.service"}, Actions: []string{Start, Stop, Restart, Reload}, Link: "/email"},
	{Key: "opendkim", Label: "OpenDKIM", Group: GroupService, Units: []string{"opendkim.service"}, Actions: []string{Start, Stop, Restart}, Link: "/email"},
}

// PhpMyAdmin is not a unit: it is files and a pool of PHP-FPM, installed by the panel and opened from the Databases tab.
const PhpMyAdmin = "phpmyadmin"

var phpActions = []string{Start, Stop, Restart, Reload}

// PHPKey is the key of a PHP version, such as "php-8.4"; NodeKey that of a Node.js major version, such as "node-22".
func PHPKey(v string) string  { return "php-" + v }
func NodeKey(v string) string { return "node-" + v }

func phpUnit(v string) string { return "php" + v + "-fpm.service" }

func find(key string) (Def, bool) {
	for _, d := range Defs {
		if d.Key == key {
			return d, true
		}
	}
	return Def{}, false
}

func phpVersion(key string) (string, bool) {
	v, ok := strings.CutPrefix(key, "php-")
	return v, ok && site.ValidPHPVersion(v)
}

func nodeVersion(key string) (string, bool) {
	v, ok := strings.CutPrefix(key, "node-")
	return v, ok && slices.Contains(site.NodeVersions, v)
}

// ValidAction reports whether action may be done to the service named by key. The check is on the shape only; whether the service
// is installed is found out when it is done.
func ValidAction(key, action string) error {
	var allowed []string
	if _, ok := phpVersion(key); ok {
		allowed = phpActions
	} else if d, ok := find(key); ok {
		allowed = d.Actions
	} else {
		return fmt.Errorf("service %q is not one that the panel manages", key)
	}
	if !slices.Contains(allowed, action) {
		return fmt.Errorf("a service cannot be asked to %q here", action)
	}
	return nil
}

// ValidInstall reports whether key is something the panel can install: the database browser, a PHP or Node.js version, or a database
// server.
func ValidInstall(key string) error {
	if key == PhpMyAdmin || key == "mariadb" || key == "mysql" {
		return nil
	}
	if _, ok := phpVersion(key); ok {
		return nil
	}
	if _, ok := nodeVersion(key); ok {
		return nil
	}
	return fmt.Errorf("the panel cannot install %q", key)
}

// InstallVersion splits a "php-8.4" or "node-22" key into its kind and version.
func InstallVersion(key string) (kind, version string, ok bool) {
	if v, ok := phpVersion(key); ok {
		return GroupPHP, v, true
	}
	if v, ok := nodeVersion(key); ok {
		return GroupNode, v, true
	}
	return "", "", false
}

// Candidates returns the units a service may be, in the order to try them.
func Candidates(key string) ([]string, bool) {
	if v, ok := phpVersion(key); ok {
		return []string{phpUnit(v)}, true
	}
	if d, ok := find(key); ok {
		return d.Units, true
	}
	return nil, false
}

// Item is one row of the page.
type Item struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Group       string   `json:"group"`
	Installed   bool     `json:"installed"`
	Unit        string   `json:"unit,omitempty"`
	State       string   `json:"state,omitempty"` // what systemd says: active, inactive, failed...
	Active      bool     `json:"active"`
	Actions     []string `json:"actions,omitempty"`
	Installable bool     `json:"installable"`
	Version     string   `json:"version,omitempty"`
	Needs       string   `json:"needs,omitempty"` // what is missing for an install: "panel", "database" or "legacyauth"
	Link        string   `json:"link,omitempty"`  // the page that sets this program up
}

// Probe is what Status asks of the machine, so a test can answer without one.
type Probe struct {
	State    monitor.UnitState
	Node     []string // the installed Node.js major versions
	Exists   func(path string) bool
	ReadFile func(path string) (string, error)
}

// panelConf is the file the panel's nginx block is written to; it is there on every machine the panel was installed on.
const panelConf = "/etc/nginx/conf.d/01-mpn-panel.conf"

func (p Probe) unit(key string) (id, state string) {
	cands, _ := Candidates(key)
	for _, u := range cands {
		if id, st := p.State(u); id != "" {
			return id, st
		}
	}
	return "", ""
}

// Status lists what is installed and what could be installed. Versions are listed newest first.
func Status(p Probe) []Item {
	var out []Item
	dbID := ""
	for _, d := range Defs {
		id, st := p.unit(d.Key)
		switch {
		case id != "":
			if d.Key == "database" {
				dbID = id
			}
			out = append(out, Item{Key: d.Key, Label: d.Label, Group: d.Group, Installed: true, Unit: id, State: st, Active: st == "active", Actions: d.Actions, Link: d.Link})
		case d.Absent:
			out = append(out, Item{Key: d.Key, Label: d.Label, Group: d.Group, Link: d.Link})
		}
	}
	if dbID == "" { // the only time a database server is offered: with one on the machine there is nothing to choose
		out = append(out,
			Item{Key: "mariadb", Label: "MariaDB", Group: GroupDatabase, Installable: true},
			Item{Key: "mysql", Label: "MySQL", Group: GroupDatabase, Installable: true})
	}
	for _, v := range slices.Backward(site.PHPVersions) {
		it := Item{Key: PHPKey(v), Label: "PHP-FPM " + v, Group: GroupPHP}
		if id, st := p.State(phpUnit(v)); id != "" {
			it.Installed, it.Unit, it.State, it.Active, it.Actions = true, id, st, st == "active", phpActions
		} else if v < "7.4" && dbID == "mysql.service" {
			// PHP before 7.4 cannot sign in to MySQL 8 unless the server is set up for the old password plugin, which the installer
			// does only together with the database. Adding that later is not done from here.
			it.Needs = "legacyauth"
		} else {
			it.Installable = true
		}
		out = append(out, it)
	}
	for _, v := range slices.Backward(site.NodeVersions) {
		it := Item{Key: NodeKey(v), Label: "Node.js " + v, Group: GroupNode}
		if slices.Contains(p.Node, v) {
			it.Installed = true
		} else {
			it.Installable = true
		}
		out = append(out, it)
	}
	pma := Item{Key: PhpMyAdmin, Label: "phpMyAdmin", Group: GroupTool}
	if p.Exists(installer.PMAVersionFile) {
		pma.Installed = true
		if v, err := p.ReadFile(installer.PMAVersionFile); err == nil {
			pma.Version = strings.TrimSpace(v)
		}
	} else if !p.Exists(panelConf) {
		pma.Needs = "panel"
	} else if dbID == "" {
		pma.Needs = "database"
	} else {
		pma.Installable = true
	}
	return append(out, pma)
}
