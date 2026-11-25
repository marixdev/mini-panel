package installer

import (
	"strings"
	"testing"

	"mpn/internal/osinfo"
	"mpn/internal/tune"
)

var bookworm = osinfo.Target{ID: "debian", Version: "12", Codename: "bookworm"}

func opts(engine string, swap int) Options {
	return Options{Target: bookworm, Profile: tune.New(tune.Input{RAMMB: 1024, CPUs: 1}), CurrentSwapMB: swap, Engine: engine}
}

func names(p Plan) string {
	var n []string
	for _, s := range p {
		n = append(n, s.Name)
	}
	return strings.Join(n, ",")
}

// scripts is every command of the plan in full; Plan.String cuts a multi-line script after its first line.
func scripts(p Plan) string {
	var b strings.Builder
	for _, s := range p {
		for _, a := range s.Actions {
			b.WriteString(strings.Join(a.Args, " ") + "\n")
		}
	}
	return b.String()
}

func TestStepOrderWithSwap(t *testing.T) {
	p, err := Build(opts("mariadb", 0))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := names(p), "base packages,swap,journald limit,nginx,nginx pagespeed,database mariadb"; got != want {
		t.Errorf("steps=%s want %s", got, want)
	}
	if !strings.Contains(scripts(p), `fallocate -l 1024M "$f"`) {
		t.Errorf("swap size missing:\n%s", p)
	}
}

// A /swapfile that is already active swap is left alone: the script picks another name (fallocate on it says "Text file busy").
func TestSwapScriptSkipsAnActiveSwapfile(t *testing.T) {
	s := swapScript(512)
	for _, want := range []string{"swapon --show=NAME", "/swapfile-mpn", `grep -qx "$c"`, "dd if=/dev/zero", "count=512"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q:\n%s", want, s)
		}
	}
}

func TestSwapSkippedWhenEnough(t *testing.T) {
	p, _ := Build(opts("", 1024))
	if got, want := names(p), "base packages,journald limit,nginx,nginx pagespeed"; got != want {
		t.Errorf("steps=%s want %s", got, want)
	}
}

func TestSwapOnlyTopsUp(t *testing.T) {
	p, _ := Build(opts("", 256))
	if s := scripts(p); !strings.Contains(s, "fallocate -l 768M") || !strings.Contains(s, "count=768") {
		t.Errorf("want top-up of 768M:\n%s", p)
	}
}

func TestNginxConfUsesProfile(t *testing.T) {
	c := nginxConf(tune.New(tune.Input{RAMMB: 1024, CPUs: 4}))
	if !strings.Contains(c, "worker_processes 2;") || !strings.Contains(c, "worker_connections 1024;") {
		t.Errorf("conf:\n%s", c)
	}
}

func TestMySQLStep(t *testing.T) {
	p, err := Build(opts("mysql", 1024))
	if err != nil {
		t.Fatal(err)
	}
	s := p.String()
	for _, want := range []string{"mysql-community-server", "/etc/apt/sources.list.d/mysql.sources", "/etc/mysql/conf.d/99-mpn.cnf"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q:\n%s", want, s)
		}
	}
}

func TestBadInputs(t *testing.T) {
	if _, err := Build(opts("postgres", 0)); err == nil {
		t.Error("unknown engine must fail")
	}
	o := opts("mysql", 0)
	o.DBSeries = "5.7"
	if _, err := Build(o); err == nil {
		t.Error("unknown series must fail")
	}
}
