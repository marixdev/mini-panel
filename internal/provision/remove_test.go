package provision

import (
	"strings"
	"testing"

	"mpn/internal/site"
)

func TestRemovePHPKeepsAccountByDefault(t *testing.T) {
	s := base(site.PHP)
	s.PHPVersion = "8.4"
	p, err := RemovePlan(s, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(p), "nginx vhost,php-fpm pool"; got != want {
		t.Errorf("steps=%s want %s", got, want)
	}
	out := p.String()
	for _, want := range []string{"rm -f /etc/nginx/sites-enabled/demo.conf", "rm -f /etc/php/8.4/fpm/pool.d/demo.conf", "disable --now php8.4-fpm"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "userdel") {
		t.Errorf("account must be kept without purge:\n%s", out)
	}
	if strings.Index(out, "sites-enabled/demo.conf") > strings.Index(out, "pool.d/demo.conf") {
		t.Error("the vhost must go before the pool so no request reaches a removed pool")
	}
}

func TestRemoveStopsTheBackgroundServices(t *testing.T) {
	s := base(site.PHP)
	s.PHPVersion = "8.4"
	p, err := RemovePlan(s, false)
	if err != nil {
		t.Fatal(err)
	}
	out := p.String()
	for _, want := range []string{"app-demo-bg-*.service", "daemon-reload"} {
		if !strings.Contains(out, want) {
			t.Errorf("the removal must stop the background services (%q missing):\n%s", want, out)
		}
	}
}

func TestRemoveNodeAndPurge(t *testing.T) {
	s := base(site.Node)
	s.NodePort, s.NodeVersion, s.Entry = 20000, "22", "server.js"
	p, err := RemovePlan(s, true)
	if err != nil {
		t.Fatal(err)
	}
	out := p.String()
	for _, want := range []string{"systemctl disable --now app-demo.service", "rm -f /etc/systemd/system/app-demo.service", "pkill -u demo", "userdel -r demo"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestRemoveRejectsInvalidSite(t *testing.T) {
	s := base(site.Static)
	s.Name = "../etc"
	if p, err := RemovePlan(s, true); err == nil || p != nil {
		t.Errorf("unsafe name must be rejected, plan=%v err=%v", p, err)
	}
}
