package services

import (
	"testing"

	"mpn/internal/installer"
)

func probe(units map[string]string, files map[string]string, node ...string) Probe {
	return Probe{
		State: func(u string) (string, string) {
			if st, ok := units[u]; ok {
				return u, st
			}
			return "", ""
		},
		Node:     node,
		Exists:   func(p string) bool { _, ok := files[p]; return ok },
		ReadFile: func(p string) (string, error) { return files[p], nil },
	}
}

func TestADatabaseServerOffersTheVersionsOfThisRelease(t *testing.T) {
	p := probe(map[string]string{"nginx.service": "active"}, nil)
	p.Series = func(engine string) []string {
		return map[string][]string{"mariadb": {"10.11", "11.8"}, "mysql": {"8.0", "8.4"}}[engine]
	}
	items := Status(p)
	for engine, want := range map[string]string{"mariadb": "11.8", "mysql": "8.4"} {
		it, ok := byKey(items, engine)
		if !ok || len(it.Series) == 0 || it.DefaultSeries != want {
			t.Errorf("%s: %+v, want a list with default %s", engine, it, want)
		}
	}
	p.Series = nil
	if it, _ := byKey(Status(p), "mariadb"); len(it.Series) != 0 {
		t.Errorf("without a release to ask there is no choice to show: %+v", it)
	}
	// With a server on the machine nothing is offered, so no version either.
	if _, ok := byKey(Status(probe(map[string]string{"mariadb.service": "active"}, nil)), "mysql"); ok {
		t.Error("a machine with MariaDB is not offered MySQL")
	}
}

func TestOnlyTheShapeOfAVersionIsAccepted(t *testing.T) {
	for _, s := range []string{"11.8", "8.4", "10.11", "12.3"} {
		if err := ValidSeries(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"", "11", "11.8.1", "latest", "11.8; id", "-1.2", "11.8\n", "../8.4"} {
		if ValidSeries(s) == nil {
			t.Errorf("%q must be refused", s)
		}
	}
}

func byKey(items []Item, key string) (Item, bool) {
	for _, it := range items {
		if it.Key == key {
			return it, true
		}
	}
	return Item{}, false
}

func TestOnlyTheListedServicesAndActionsAreAccepted(t *testing.T) {
	good := [][2]string{{"nginx", Reload}, {"nginx", Restart}, {"database", Stop}, {"php-8.4", Reload}, {"fail2ban", Reload}, {"dovecot", Restart}}
	for _, g := range good {
		if err := ValidAction(g[0], g[1]); err != nil {
			t.Errorf("%v: %v", g, err)
		}
	}
	bad := [][2]string{
		{"nginx", Stop},                  // stopping nginx would cut the panel off
		{"mpn-web", Stop}, {"ssh", Stop}, // not in the list at all
		{"redis", Start}, {"memcached", Start}, // not something mpn installs or manages
		{"opendkim", Reload},   // OpenDKIM has no reload here
		{"php-9.9", Restart},   // not a PHP version the panel knows
		{"php-8.4; id", Start}, // a key is never used as a command
		{"node-22", Start},     // Node.js is files, not a unit
		{"../nginx", Restart}, {"", Start}, {"nginx", "enable"}, {"nginx", ""},
	}
	for _, b := range bad {
		if ValidAction(b[0], b[1]) == nil {
			t.Errorf("%v must be refused", b)
		}
	}
}

func TestOnlyWhatTheInstallerSupportsCanBeInstalled(t *testing.T) {
	for _, k := range []string{PhpMyAdmin, "mariadb", "mysql", "php-5.6", "php-7.0", "php-8.5", "node-18", "node-24"} {
		if err := ValidInstall(k); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
	for _, k := range []string{"redis", "memcached", "nginx", "postfix", "php-5.4", "php-9.0", "node-16", "node-22; id", "", "php-"} {
		if ValidInstall(k) == nil {
			t.Errorf("%q must be refused", k)
		}
	}
}

func TestEveryPHPAndNodeVersionTheInstallerOffersIsListed(t *testing.T) {
	items := Status(probe(nil, nil))
	for _, k := range []string{"php-7.0", "php-7.4", "php-8.0", "php-8.4", "php-8.5", "node-18", "node-20", "node-22", "node-24"} {
		it, ok := byKey(items, k)
		if !ok || it.Installed || !it.Installable {
			t.Errorf("%s: %+v", k, it)
		}
	}
	// newest first inside a group
	var php []string
	for _, it := range items {
		if it.Group == GroupPHP {
			php = append(php, it.Key)
		}
	}
	if len(php) != 12 || php[0] != "php-8.5" || php[11] != "php-5.6" {
		t.Errorf("php order: %v", php)
	}
}

func TestStatusShowsWhatIsInstalledAndWhatCouldBe(t *testing.T) {
	p := probe(map[string]string{"nginx.service": "active", "mariadb.service": "inactive", "php8.4-fpm.service": "active", "php8.3-fpm.service": "inactive"},
		map[string]string{"/etc/nginx/conf.d/01-mpn-panel.conf": ""}, "22")
	items := Status(p)
	if it, ok := byKey(items, "nginx"); !ok || !it.Installed || !it.Active || len(it.Actions) != 3 {
		t.Errorf("nginx: %+v", it)
	}
	if it, _ := byKey(items, "database"); !it.Installed || it.Active || it.State != "inactive" {
		t.Errorf("a stopped database is installed and not active: %+v", it)
	}
	if _, ok := byKey(items, "mariadb"); ok {
		t.Error("with a database server on the machine there is nothing to choose")
	}
	if it, _ := byKey(items, "php-8.4"); !it.Installed || !it.Active || len(it.Actions) != 4 {
		t.Errorf("php 8.4: %+v", it)
	}
	if it, _ := byKey(items, "php-8.3"); !it.Installed || it.Active {
		t.Errorf("an unused version is installed and stopped: %+v", it)
	}
	if it, _ := byKey(items, "php-8.2"); it.Installed || !it.Installable {
		t.Errorf("php 8.2: %+v", it)
	}
	if it, _ := byKey(items, "node-22"); !it.Installed || it.Installable || it.Unit != "" {
		t.Errorf("node 22 is files, not a unit: %+v", it)
	}
	if it, _ := byKey(items, "node-20"); it.Installed || !it.Installable {
		t.Errorf("node 20: %+v", it)
	}
	if _, ok := byKey(items, "redis"); ok {
		t.Error("redis is not something mpn manages")
	}
}

func TestMailAndFail2banAreShownWithALinkToTheirPage(t *testing.T) {
	items := Status(probe(nil, nil))
	if it, ok := byKey(items, "fail2ban"); !ok || it.Installed || it.Installable || it.Link != "/security" {
		t.Errorf("fail2ban: %+v", it)
	}
	if it, ok := byKey(items, "postfix"); !ok || it.Installed || it.Link != "/email" {
		t.Errorf("postfix: %+v", it)
	}
	if _, ok := byKey(items, "dovecot"); ok {
		t.Error("the other mail programs show once the mail server is on")
	}
	on := Status(probe(map[string]string{"postfix.service": "active", "dovecot.service": "active", "opendkim.service": "active"}, nil))
	for _, k := range []string{"postfix", "dovecot", "opendkim"} {
		if it, ok := byKey(on, k); !ok || !it.Installed || len(it.Actions) == 0 {
			t.Errorf("%s: %+v", k, it)
		}
	}
}

func TestADatabaseServerIsOfferedOnlyWhenThereIsNone(t *testing.T) {
	none := Status(probe(nil, nil))
	for _, k := range []string{"mariadb", "mysql"} {
		if it, ok := byKey(none, k); !ok || !it.Installable {
			t.Errorf("%s: %+v", k, it)
		}
	}
}

func TestOldPHPIsNotOfferedOnMySQL8(t *testing.T) {
	items := Status(probe(map[string]string{"mysql.service": "active"}, nil))
	if it, _ := byKey(items, "php-7.3"); it.Installable || it.Needs != "legacyauth" {
		t.Errorf("php 7.3 cannot sign in to MySQL 8: %+v", it)
	}
	if it, _ := byKey(items, "php-7.4"); !it.Installable {
		t.Errorf("php 7.4 can: %+v", it)
	}
	maria := Status(probe(map[string]string{"mariadb.service": "active"}, nil))
	if it, _ := byKey(maria, "php-7.3"); !it.Installable {
		t.Errorf("MariaDB has no such limit: %+v", it)
	}
}

func TestPhpMyAdminNeedsThePanelAndADatabase(t *testing.T) {
	noPanel := Status(probe(map[string]string{"mariadb.service": "active"}, nil))
	if it, _ := byKey(noPanel, PhpMyAdmin); it.Installable || it.Needs != "panel" {
		t.Errorf("%+v", it)
	}
	noDB := Status(probe(nil, map[string]string{"/etc/nginx/conf.d/01-mpn-panel.conf": ""}))
	if it, _ := byKey(noDB, PhpMyAdmin); it.Installable || it.Needs != "database" {
		t.Errorf("%+v", it)
	}
	ready := Status(probe(map[string]string{"mariadb.service": "active"}, map[string]string{"/etc/nginx/conf.d/01-mpn-panel.conf": ""}))
	if it, _ := byKey(ready, PhpMyAdmin); !it.Installable || it.Needs != "" {
		t.Errorf("%+v", it)
	}
	done := Status(probe(nil, map[string]string{installer.PMAVersionFile: "5.2.3\n"}))
	if it, _ := byKey(done, PhpMyAdmin); !it.Installed || it.Installable || it.Version != "5.2.3" {
		t.Errorf("an installed browser shows its version: %+v", it)
	}
}

func TestTwoNamesOfOneDatabaseAreFoundInOrder(t *testing.T) {
	p := probe(map[string]string{"mysql.service": "active"}, nil)
	if it, _ := byKey(Status(p), "database"); it.Unit != "mysql.service" {
		t.Errorf("%+v", it)
	}
}
