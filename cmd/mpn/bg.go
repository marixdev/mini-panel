package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"mpn/internal/background"
	"mpn/internal/installer"
	"mpn/internal/site"
	"mpn/internal/store"
)

func bgUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn bg <list|add|set|remove|restart|sync> -site NAME [flags]   (sync also takes -all; add -h for the flags)")
	os.Exit(2)
}

// bgSystemctl runs systemctl and returns what it printed. A missing systemctl or an error gives "". It is a variable so that
// tests can answer for systemd.
var bgSystemctl = func(args ...string) string {
	o, _ := sh(5*time.Second, "systemctl", args...)
	return o
}

// runBg manages the background services of a website. Each one is a systemd unit that runs as the website's own account.
// mpn keeps them in its database and writes one unit file per service. The unit file is written, and the service started or
// stopped, before the database changes, so what the database says is what runs.
func runBg(args []string) {
	if len(args) == 0 {
		bgUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("bg "+sub, flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	siteName := fs.String("site", "", "site name")
	id := fs.Int64("id", 0, "service number, for set, remove and restart")
	stdin := fs.Bool("stdin", false, "add and set: read {name, args, enabled, memory_mb} as JSON on standard input, so the arguments are not in argv")
	asJSON := fs.Bool("json", false, "list: print JSON")
	all := fs.Bool("all", false, "sync: every site instead of -site")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])
	st := openStore(*db)
	defer st.Close()
	if sub == "sync" {
		bgSync(st, *siteName, *all, *apply)
		return
	}
	s, err := st.GetSite(*siteName)
	fail(err)
	fail(background.Supported(s.Kind))
	list, err := st.BgServices(s.Name)
	fail(err)

	switch sub {
	case "list":
		bgStates(list, s.Name)
		if *asJSON {
			printJSON(list)
			return
		}
		for _, v := range list {
			fmt.Printf("%d\t%s\t%v\t%d\t%s\t%s\n", v.ID, v.Name, v.Enabled, v.MemoryMB, v.Args, v.State)
		}
		return
	case "restart":
		if !hasBg(list, *id) {
			fail(fmt.Errorf("site %q has no background service %d", s.Name, *id))
		}
		if !*apply {
			fmt.Printf("# checked only. The service %s would be restarted\n", background.UnitName(s.Name, *id))
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(installer.Plan{{Name: "background service", Actions: []installer.Action{
			installer.Run("systemctl", "restart", background.UnitName(s.Name, *id)),
		}}}))
		fmt.Printf("service %d of site %q restarted\n", *id, s.Name)
		return
	case "add", "set", "remove":
	default:
		bgUsage()
	}

	var in background.Input
	if sub == "add" || sub == "set" {
		if !*stdin {
			fail(fmt.Errorf("%s reads its settings from standard input: pass -stdin", sub))
		}
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 16<<10))
		fail(err)
		if err := json.Unmarshal(raw, &in); err != nil {
			fail(fmt.Errorf("the input is not the JSON that mpn expects: %w", err))
		}
	}

	switch sub {
	case "add":
		if len(list) >= background.MaxServices {
			fail(fmt.Errorf("a site may have at most %d background services", background.MaxServices))
		}
		fail(background.ValidService(s.Kind, in))
		n, err := st.NextBgServiceID()
		fail(err)
		*id = n
		list = append(list, background.Service{ID: n, Name: in.Name, Args: in.Args, Enabled: in.Enabled, MemoryMB: in.MemoryMB})
	case "set":
		if !hasBg(list, *id) {
			fail(fmt.Errorf("site %q has no background service %d", s.Name, *id))
		}
		fail(background.ValidService(s.Kind, in))
		for i := range list {
			if list[i].ID == *id {
				list[i] = background.Service{ID: *id, Name: in.Name, Args: in.Args, Enabled: in.Enabled, MemoryMB: in.MemoryMB}
			}
		}
	case "remove":
		if !hasBg(list, *id) {
			fail(fmt.Errorf("site %q has no background service %d", s.Name, *id))
		}
		out := list[:0:0]
		for _, v := range list {
			if v.ID != *id {
				out = append(out, v)
			}
		}
		list = out
	}

	// Each service is checked against the kind of the site before anything is written.
	plan, err := bgPlan(s, list, sub, *id)
	fail(err)
	if !*apply {
		fmt.Print(plan)
		fmt.Println("# checked only. Nothing was changed. With -apply the unit files are written and then the database is stored.")
		return
	}
	requireLinuxRoot()
	fail(runPlanQuiet(plan))
	switch sub {
	case "add":
		fail(st.AddBgService(s.Name, s.Kind, *id, in))
		fmt.Printf("service %d added to site %q\n", *id, s.Name)
	case "set":
		fail(st.UpdateBgService(s.Name, s.Kind, *id, in))
		fmt.Printf("service %d of site %q updated\n", *id, s.Name)
	case "remove":
		fail(st.DeleteBgService(s.Name, *id))
		fmt.Printf("service %d removed from site %q\n", *id, s.Name)
	}
}

// bgStates fills in the state of each service from systemd: active, failed or inactive, or unknown when systemd does not answer.
func bgStates(list []background.Service, siteName string) {
	for i := range list {
		list[i].State = background.StateOf(bgSystemctl("is-active", background.UnitName(siteName, list[i].ID)))
	}
}

// bgSync rewrites every unit of the site, or of every site with all, from the database, and removes the unit files that no
// service of the database owns any more. Without apply it only prints the plan.
func bgSync(st *store.Store, siteName string, all, apply bool) {
	var sites []site.Site
	if all {
		list, err := st.ListSites()
		fail(err)
		sites = list
	} else {
		if siteName == "" {
			fail(fmt.Errorf("sync needs -site NAME or -all"))
		}
		s, err := st.GetSite(siteName)
		fail(err)
		sites = []site.Site{s}
	}
	plan, done, err := bgSyncPlan(st, sites, background.UnitDir)
	fail(err)
	if !apply {
		fmt.Print(plan)
		fmt.Printf("# checked only. Nothing was changed. With -apply the services of %d site(s) are written again.\n", len(done))
		return
	}
	requireLinuxRoot()
	fail(runPlanQuiet(plan))
	fmt.Printf("services of %d site(s) written again\n", len(done))
}

// bgSyncPlan builds the plan of a full resync. Stray unit files in dir are stopped and deleted, the units of every site that
// can have services are written, and the services are started or stopped. Sites of other kinds are skipped. The names of the
// sites that were planned are returned.
func bgSyncPlan(st *store.Store, sites []site.Site, dir string) (installer.Plan, []string, error) {
	var stops, writes, runs []installer.Action
	var done []string
	for _, s := range sites {
		if background.Supported(s.Kind) != nil {
			continue
		}
		list, err := st.BgServices(s.Name)
		if err != nil {
			return nil, nil, err
		}
		keep := map[int64]bool{}
		for _, v := range list {
			keep[v.ID] = true
		}
		strays, err := background.StrayUnits(dir, s.Name, keep)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range strays {
			stops = append(stops, installer.Run("systemctl", "disable", "--now", filepath.Base(p)), installer.Run("rm", "-f", p))
		}
		w, r, err := bgSteps(s, list)
		if err != nil {
			return nil, nil, err
		}
		writes = append(writes, w...)
		runs = append(runs, r...)
		done = append(done, s.Name)
	}
	actions := append(stops, writes...)
	actions = append(actions, bgReload())
	actions = append(actions, runs...)
	return installer.Plan{{Name: "background services", Actions: actions}}, done, nil
}

func hasBg(list []background.Service, id int64) bool {
	for _, v := range list {
		if v.ID == id {
			return true
		}
	}
	return false
}

// bgPlan writes the unit of every service that is left after the change and removes the unit of a removed one. Each unit is
// checked against the site before it is written, so a service that no longer fits the site stops the whole change.
func bgPlan(s site.Site, list []background.Service, sub string, id int64) (installer.Plan, error) {
	var actions []installer.Action
	if sub == "remove" {
		actions = append(actions, installer.Run("systemctl", "disable", "--now", background.UnitName(s.Name, id)), installer.Run("rm", "-f", background.UnitFile(s.Name, id)))
	}
	writes, runs, err := bgSteps(s, list)
	if err != nil {
		return nil, err
	}
	actions = append(actions, writes...)
	actions = append(actions, bgReload())
	actions = append(actions, runs...)
	return installer.Plan{{Name: "background services of " + s.Name, Actions: actions}}, nil
}

// bgSteps returns the actions that write the unit files of the services of a site, and the actions that start, stop or enable
// them. The caller runs the daemon-reload between the two groups, so a restarted unit reads the file that was just written.
func bgSteps(s site.Site, list []background.Service) (writes, runs []installer.Action, err error) {
	for _, v := range list {
		content, err := background.Unit(s, v)
		if err != nil {
			return nil, nil, err
		}
		u := background.UnitName(s.Name, v.ID)
		writes = append(writes, installer.WriteFile(background.UnitFile(s.Name, v.ID), content, 0o644))
		if v.Enabled {
			// Not "enable --now": a service that is already running would keep its old arguments and memory limit.
			runs = append(runs, installer.Run("systemctl", "enable", u), installer.Run("systemctl", "restart", u))
		} else {
			runs = append(runs, installer.Run("systemctl", "disable", "--now", u))
		}
	}
	return writes, runs, nil
}

func bgReload() installer.Action { return installer.Run("systemctl", "daemon-reload") }

// bgVersionPlan returns the steps that rewrite the units of every service of a site whose runtime version changed. A new
// version changes the interpreter of each service, so the units are written again. Nothing is returned when no version changed.
func bgVersionPlan(old, cur site.Site, list []background.Service) (installer.Plan, error) {
	if len(list) == 0 || (cur.PHPVersion == old.PHPVersion && cur.NodeVersion == old.NodeVersion && cur.PyVersion == old.PyVersion) {
		return nil, nil
	}
	return bgPlan(cur, list, "sync", 0)
}
