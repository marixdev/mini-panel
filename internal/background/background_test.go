package background

import (
	"reflect"
	"strings"
	"testing"

	"mpn/internal/site"
)

func TestParseArgsSplitsAndQuotes(t *testing.T) {
	for in, want := range map[string][]string{
		"queue:work --sleep=3":       {"queue:work", "--sleep=3"},
		"  worker.js   --port 8080 ": {"worker.js", "--port", "8080"},
		`-m celery -A app worker`:    {"-m", "celery", "-A", "app", "worker"},
		`run.py 'a b' "c d"`:         {"run.py", "a b", "c d"},
		`run.py "say \"hi\""`:        {"run.py", `say "hi"`},
		`run.py a\ b`:                {"run.py", "a b"},
		`run.py 'a\b'`:               {"run.py", `a\b`},
		`run.py "a\\b"`:              {"run.py", `a\b`},
		`run.py ''`:                  {"run.py", ""},
		`run.py "" x`:                {"run.py", "", "x"},
		"run.py\tx":                  {"run.py", "x"},
	} {
		got, err := ParseArgs(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ParseArgs(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestParseArgsRefusesInjectionAndBrokenInput(t *testing.T) {
	for name, in := range map[string]string{
		"semicolon":            "a;rm",
		"pipe":                 "a|b",
		"ampersand":            "a&b",
		"redirect":             "a>b",
		"input redirect":       "a<b",
		"dollar":               "$HOME",
		"command substitution": "$(x)",
		"backticks":            "`x`",
		"parentheses":          "a(b)",
		"braces":               "a{b}",
		"newline":              "a\nb",
		"carriage return":      "a\rb",
		"nul":                  "a\x00b",
		"other control":        "a\x01b",
		"open quote":           `run.py "abc`,
		"open single quote":    `run.py 'abc`,
		"trailing backslash":   `run.py a\`,
		"bad utf-8":            "a\xffb",
		"too long":             strings.Repeat("a", MaxArgsText+1),
	} {
		if _, err := ParseArgs(in); err == nil {
			t.Errorf("%s: %q must be refused", name, in)
		}
	}
}

func TestParseArgsRefusesForbiddenCharactersInsideQuotes(t *testing.T) {
	// A forbidden character is refused even when quoted, so no quoting trick reaches a shell.
	for _, in := range []string{`'a;b'`, `"$(x)"`, `"a|b"`} {
		if _, err := ParseArgs(in); err == nil {
			t.Errorf("%q must be refused even inside quotes", in)
		}
	}
}

func TestValidArgsRules(t *testing.T) {
	ok := []struct {
		kind site.Kind
		args []string
	}{
		{site.PHP, []string{"artisan", "queue:work", "--sleep=3"}},
		{site.Node, []string{"worker.js"}},
		{site.Node, []string{"lib/bot.js", "--x"}},
		{site.Python, []string{"worker.py"}},
		{site.Python, []string{"-m", "celery", "-A", "app", "worker"}},
	}
	for _, c := range ok {
		if err := ValidArgs(c.kind, c.args); err != nil {
			t.Errorf("%s %q: %v", c.kind, c.args, err)
		}
	}
	bad := []struct {
		name string
		kind site.Kind
		args []string
	}{
		{"no args", site.PHP, nil},
		{"too many args", site.PHP, make([]string, MaxArgs+1)},
		{"empty arg", site.PHP, []string{""}},
		{"long arg", site.PHP, []string{strings.Repeat("a", MaxArgLen+1)}},
		{"option first for php", site.PHP, []string{"-v"}},
		{"option first for node", site.Node, []string{"--inspect", "x.js"}},
		{"-m for node", site.Node, []string{"-m", "x"}},
		{"-m without module", site.Python, []string{"-m"}},
		{"-m followed by option", site.Python, []string{"-m", "-x"}},
		{"absolute path", site.Node, []string{"/usr/bin/x"}},
		{"dot dot", site.Node, []string{"../x.js"}},
		{"dot dot inside", site.Python, []string{"a/../../x.py"}},
	}
	for _, c := range bad {
		if err := ValidArgs(c.kind, c.args); err == nil {
			t.Errorf("%s must be refused", c.name)
		}
	}
	// A name such as "x..y" is a file name, not a parent segment.
	if err := ValidArgs(site.Node, []string{"x..y.js"}); err != nil {
		t.Errorf("x..y.js: %v", err)
	}
}

func TestNames(t *testing.T) {
	for _, ok := range []string{"queue", "Queue Worker", "bot_1.main-2", "9lives"} {
		if err := ValidName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", " x", "-x", "_x", "a;b", "a/b", "a\nb", strings.Repeat("a", 49)} {
		if err := ValidName(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestMemory(t *testing.T) {
	for _, ok := range []int{0, 32, 256, 4096} {
		if err := ValidMemory(ok); err != nil {
			t.Errorf("%d: %v", ok, err)
		}
	}
	for _, bad := range []int{-1, 1, 31, 4097} {
		if err := ValidMemory(bad); err == nil {
			t.Errorf("%d must be refused", bad)
		}
	}
}

func TestQuoteEscapesSystemdSpecials(t *testing.T) {
	for in, want := range map[string]string{
		`plain`: `"plain"`,
		`a"b`:   `"a\"b"`,
		`a\b`:   `"a\\b"`,
		`100%`:  `"100%%"`,
		`%h/x`:  `"%%h/x"`,
	} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func phpSite() site.Site {
	return site.Site{Name: "shop", Kind: site.PHP, User: "shop", Root: "/home/shop/htdocs", Hostnames: []string{"shop.example.com"}, PHPVersion: "8.4"}
}

func nodeSite() site.Site {
	return site.Site{Name: "api", Kind: site.Node, User: "api", Root: "/home/api/htdocs", Hostnames: []string{"api.example.com"}, NodePort: 20001, NodeVersion: "22", Entry: "server.js"}
}

func pySite() site.Site {
	return site.Site{Name: "ml", Kind: site.Python, User: "ml", Root: "/home/ml/htdocs", Hostnames: []string{"ml.example.com"}, NodePort: 20002, PyVersion: "3.12", Entry: "app.py"}
}

func TestRenderPHP(t *testing.T) {
	out, err := Unit(phpSite(), Service{ID: 4, Name: "queue", Args: "artisan queue:work --sleep=3", Enabled: true, MemoryMB: 256})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Description=Background service queue of website shop",
		"User=shop",
		"WorkingDirectory=/home/shop/htdocs",
		`ExecStart="/usr/bin/php8.4" "artisan" "queue:work" "--sleep=3"`,
		"Restart=always", "RestartSec=3", "NoNewPrivileges=yes", "PrivateTmp=yes", "ProtectSystem=full",
		"MemoryMax=256M", "WantedBy=multi-user.target",
	} {
		if !strings.Contains(out, want) && !(strings.Contains(want, "php8.4") && strings.Contains(out, "php8.4")) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "After=app-shop.service") {
		t.Error("a php site has no app unit to wait for")
	}
}

func TestRenderNodeAndPython(t *testing.T) {
	out, err := Unit(nodeSite(), Service{ID: 1, Name: "bot", Args: "bot.js", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"After=app-api.service", "Wants=app-api.service", "WorkingDirectory=/home/api/htdocs",
		"Environment=NODE_ENV=production", `ExecStart="/opt/node/22/bin/node" "bot.js"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("node: missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "MemoryMax") {
		t.Error("no memory limit was asked for")
	}

	out, err = Unit(pySite(), Service{ID: 2, Name: "celery", Args: "-m celery -A app worker", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"After=app-ml.service", "WorkingDirectory=/home/ml/htdocs", "Environment=PYTHONUNBUFFERED=1",
		`ExecStart="/home/ml/venv/bin/python" "-m" "celery" "-A" "app" "worker"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("python: missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderEscapesPercentAndQuotesInArguments(t *testing.T) {
	out, err := Unit(nodeSite(), Service{ID: 1, Name: "x", Args: `run.js "a%b" 'q\"'`, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `ExecStart="/opt/node/22/bin/node" "run.js" "a%%b" "q\\\""`) {
		t.Errorf("escaping is wrong:\n%s", out)
	}
}

func TestRenderRefusesStaticAndProxy(t *testing.T) {
	for _, s := range []site.Site{
		{Name: "st", Kind: site.Static, User: "st", Root: "/home/st/htdocs", Hostnames: []string{"st.example.com"}},
		{Name: "px", Kind: site.Proxy, User: "px", Root: "/home/px/htdocs", Hostnames: []string{"px.example.com"}, Upstream: "http://127.0.0.1:9000"},
	} {
		if _, err := Unit(s, Service{ID: 1, Name: "x", Args: "a.js", Enabled: true}); err == nil {
			t.Errorf("%s site must be refused", s.Kind)
		}
		if err := Supported(s.Kind); err == nil {
			t.Errorf("Supported(%s) must refuse", s.Kind)
		}
	}
}

func TestUnitNamesAndRemoveScript(t *testing.T) {
	if got := UnitName("shop", 7); got != "app-shop-bg-7.service" {
		t.Errorf("%s", got)
	}
	if !strings.Contains(RemoveScript("shop"), "app-shop-bg-*.service") {
		t.Error("the cleanup must match the service units of the site")
	}
}

func TestStateOfMapsSystemctlAnswers(t *testing.T) {
	for in, want := range map[string]string{
		"active\n":       "active",
		"activating":     "active",
		"reloading":      "active",
		"failed":         "failed",
		"inactive":       "inactive",
		"deactivating\n": "inactive",
		"":               "unknown",
		"unknown":        "unknown",
		"garbage":        "unknown",
	} {
		if got := StateOf(in); got != want {
			t.Errorf("StateOf(%q) = %q, want %q", in, got, want)
		}
	}
}
