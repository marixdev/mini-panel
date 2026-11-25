package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mpn/internal/background"
	"mpn/internal/installer"
	"mpn/internal/site"
	"mpn/internal/store"
)

func bgTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "mpn.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func bgTestNode(name string, port int) site.Site {
	return site.Site{Name: name, Kind: site.Node, User: name, Root: "/home/" + name + "/htdocs", Hostnames: []string{name + ".example.com"}, NodePort: port, NodeVersion: "22", Entry: "server.js"}
}

func bgTestPHP(name string) site.Site {
	return site.Site{Name: name, Kind: site.PHP, User: name, Root: "/home/" + name + "/htdocs", Hostnames: []string{name + ".example.com"}, PHPVersion: "8.4"}
}

func bgTestIn(name string) background.Input {
	return background.Input{Name: name, Args: "worker.js", Enabled: true, MemoryMB: 128}
}

// planText lists every action of a plan as "args" or "write path", so a test can search for the units it expects.
func planText(p installer.Plan) []string {
	var out []string
	for _, step := range p {
		for _, a := range step.Actions {
			if a.Path != "" {
				out = append(out, "write "+a.Path)
			} else {
				out = append(out, strings.Join(a.Args, " "))
			}
		}
	}
	return out
}

func containsLine(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBgSyncRemovesStrayUnitsOfTheSiteOnly(t *testing.T) {
	st := bgTestStore(t)
	api := bgTestNode("api", 20001)
	if err := st.AddSite(api); err != nil {
		t.Fatal(err)
	}
	if err := st.AddBgService("api", site.Node, 1, bgTestIn("bot")); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, n := range []string{"app-api-bg-1.service", "app-api-bg-2.service", "app-api-bg-9.service", "app-shop-bg-5.service", "app-api-bg-x.service"} {
		touch(t, dir, n)
	}

	plan, done, err := bgSyncPlan(st, []site.Site{api}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 || done[0] != "api" {
		t.Fatalf("done %v", done)
	}
	lines := planText(plan)
	for _, stray := range []string{"app-api-bg-2.service", "app-api-bg-9.service"} {
		if !containsLine(lines, "rm -f "+filepath.Join(dir, stray)) {
			t.Errorf("stray %s must be removed; plan:\n%s", stray, strings.Join(lines, "\n"))
		}
		if !containsLine(lines, "systemctl disable --now "+stray) {
			t.Errorf("stray %s must be stopped and disabled", stray)
		}
	}
	for _, kept := range []string{"app-api-bg-1.service", "app-shop-bg-5.service", "app-api-bg-x.service"} {
		if containsLine(lines, "rm -f "+filepath.Join(dir, kept)) {
			t.Errorf("%s must not be removed", kept)
		}
	}
	if !containsLine(lines, "write "+background.UnitFile("api", 1)) {
		t.Error("the kept service must be written again from the database")
	}
	if lines[len(lines)-1] == "systemctl daemon-reload" {
		t.Error("the daemon-reload must come before the start and enable steps")
	}
	if !containsLine(lines, "systemctl daemon-reload") {
		t.Error("a daemon-reload is needed after the unit files change")
	}
}

func TestBgSyncAllLoopsOverSitesAndSkipsStaticAndProxy(t *testing.T) {
	st := bgTestStore(t)
	shop := bgTestPHP("shop")
	api := bgTestNode("api", 20001)
	static := site.Site{Name: "st", Kind: site.Static, User: "st", Root: "/home/st/htdocs", Hostnames: []string{"st.example.com"}}
	proxy := site.Site{Name: "px", Kind: site.Proxy, User: "px", Root: "/home/px/htdocs", Hostnames: []string{"px.example.com"}, Upstream: "http://127.0.0.1:9000"}
	for _, s := range []site.Site{shop, api, static, proxy} {
		if err := st.AddSite(s); err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
	}
	// Service numbers are global, so each one is taken from the store.
	addSvc := func(siteName string, kind site.Kind, in background.Input) int64 {
		n, err := st.NextBgServiceID()
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AddBgService(siteName, kind, n, in); err != nil {
			t.Fatal(err)
		}
		return n
	}
	shopID := addSvc("shop", site.PHP, background.Input{Name: "queue", Args: "artisan queue:work", Enabled: true})
	botID := addSvc("api", site.Node, bgTestIn("bot"))
	offID := addSvc("api", site.Node, background.Input{Name: "off", Args: "off.js", Enabled: false})
	sites, err := st.ListSites()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	touch(t, dir, "app-shop-bg-7.service")
	plan, done, err := bgSyncPlan(st, sites, dir)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(done)
	if strings.Join(done, ",") != "api,shop" {
		t.Fatalf("done %v, want api and shop only", done)
	}
	lines := planText(plan)
	for _, want := range []string{
		"write " + background.UnitFile("shop", shopID),
		"write " + background.UnitFile("api", botID),
		"write " + background.UnitFile("api", offID),
		"systemctl enable " + background.UnitName("shop", shopID),
		"systemctl restart " + background.UnitName("api", botID),
		"systemctl disable --now " + background.UnitName("api", offID),
		"systemctl disable --now app-shop-bg-7.service",
	} {
		if !containsLine(lines, want) {
			t.Errorf("missing %q in plan:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	for _, l := range lines {
		if strings.Contains(l, "app-st-bg") || strings.Contains(l, "app-px-bg") {
			t.Errorf("static or proxy site must have no unit actions: %s", l)
		}
	}
}

func TestBgSyncPlanForOneSiteWithoutServicesStillReloads(t *testing.T) {
	st := bgTestStore(t)
	api := bgTestNode("api", 20001)
	if err := st.AddSite(api); err != nil {
		t.Fatal(err)
	}
	plan, done, err := bgSyncPlan(st, []site.Site{api}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 || !containsLine(planText(plan), "systemctl daemon-reload") {
		t.Fatalf("done %v plan %v", done, planText(plan))
	}
}

func TestBgStatesFillFromSystemctlWithFakeRunner(t *testing.T) {
	answers := map[string]string{
		background.UnitName("api", 1): "active\n",
		background.UnitName("api", 2): "failed",
		background.UnitName("api", 3): "inactive",
	}
	var asked []string
	old := bgSystemctl
	bgSystemctl = func(args ...string) string {
		if len(args) != 2 || args[0] != "is-active" {
			t.Errorf("unexpected systemctl call %v", args)
		}
		asked = append(asked, args[1])
		return answers[args[1]]
	}
	t.Cleanup(func() { bgSystemctl = old })

	list := []background.Service{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}, {ID: 4, Name: "d"}}
	bgStates(list, "api")
	got := []string{list[0].State, list[1].State, list[2].State, list[3].State}
	if strings.Join(got, ",") != "active,failed,inactive,unknown" {
		t.Fatalf("states %v", got)
	}
	if len(asked) != 4 || asked[0] != background.UnitName("api", 1) {
		t.Errorf("asked %v", asked)
	}

	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"state":"failed"`) {
		t.Errorf("json must carry the state: %s", raw)
	}
}

func TestVersionChangeRewritesEveryServiceUnit(t *testing.T) {
	st := bgTestStore(t)
	api := bgTestNode("api", 20001)
	if err := st.AddSite(api); err != nil {
		t.Fatal(err)
	}
	bot := addBgSvc(t, st, "api", site.Node, bgTestIn("bot"))
	off := addBgSvc(t, st, "api", site.Node, background.Input{Name: "off", Args: "off.js", Enabled: false})
	list, err := st.BgServices("api")
	if err != nil {
		t.Fatal(err)
	}

	cur := api
	cur.NodeVersion = "24"
	plan, err := bgVersionPlan(api, cur, list)
	if err != nil {
		t.Fatal(err)
	}
	lines := planText(plan)
	for _, want := range []string{
		"write " + background.UnitFile("api", bot),
		"write " + background.UnitFile("api", off),
		"systemctl restart " + background.UnitName("api", bot),
	} {
		if !containsLine(lines, want) {
			t.Errorf("missing %q after a node version change:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if err := st.UpdateSite(cur); err != nil {
		t.Fatal(err)
	}

	same, err := bgVersionPlan(api, api, list)
	if err != nil || len(same) != 0 {
		t.Errorf("no version changed, want no bg steps: %v %v", planText(same), err)
	}
	none, err := bgVersionPlan(cur, cur, nil)
	if err != nil || len(none) != 0 {
		t.Errorf("no services, want no bg steps: %v %v", planText(none), err)
	}
}

func addBgSvc(t *testing.T, st *store.Store, siteName string, kind site.Kind, in background.Input) int64 {
	t.Helper()
	n, err := st.NextBgServiceID()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddBgService(siteName, kind, n, in); err != nil {
		t.Fatal(err)
	}
	return n
}

// An invalid service for the site's kind is refused before any unit is written: a PHP site cannot run a leading -m module,
// and a static site has no services at all.
func TestKindChecksRefuseBeforeAnyUnitIsWritten(t *testing.T) {
	dir := t.TempDir()
	php := bgTestPHP("shop")
	if err := background.ValidService(site.PHP, background.Input{Name: "w", Args: "-m celery", Enabled: true}); err == nil {
		t.Fatal("a php service that starts with -m must be refused by ValidService")
	}
	if _, _, err := bgSteps(php, []background.Service{{ID: 1, Name: "w", Args: "-m celery", Enabled: true}}); err == nil {
		t.Fatal("bgSteps must refuse a php service that starts with -m")
	}
	if err := background.Supported(site.Static); err == nil {
		t.Fatal("a static site must be refused by Supported")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("no unit file may exist: %v", entries)
	}
	static := site.Site{Name: "st", Kind: site.Static, User: "st", Root: "/home/st/htdocs", Hostnames: []string{"st.example.com"}}
	plan, done, err := bgSyncPlan(bgTestStore(t), []site.Site{static}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 0 || containsLine(planText(plan), "app-st-bg") {
		t.Errorf("a static site must produce no unit actions: %v", planText(plan))
	}
}
