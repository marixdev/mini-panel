package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"slices"

	"mpn/internal/installer"
	"mpn/internal/monitor"
	"mpn/internal/osinfo"
	"mpn/internal/services"
	"mpn/internal/site"
	"mpn/internal/tune"
	"mpn/internal/unit"
)

func servicesUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn services <status|control|install> [flags]   (add -h for the flags)")
	os.Exit(2)
}

func servicesProbe() services.Probe {
	return services.Probe{
		State: monitor.SystemdState,
		Node:  site.InstalledNode(unit.NodeRoot),
		Exists: func(p string) bool {
			_, err := os.Stat(p)
			return err == nil
		},
		ReadFile: func(p string) (string, error) {
			b, err := os.ReadFile(p)
			return string(b), err
		},
		Series: func(engine string) []string {
			t, err := thisTarget()
			if err != nil {
				return nil
			}
			return t.Series(engine)
		},
	}
}

// unitOf finds the unit that a service key stands for on this machine, or "".
func unitOf(key string) string {
	cands, _ := services.Candidates(key)
	for _, u := range cands {
		if id, _ := monitor.SystemdState(u); id != "" {
			return id
		}
	}
	return ""
}

func thisTarget() (osinfo.Target, error) {
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return osinfo.Target{}, err
	}
	return osinfo.Parse(string(raw))
}

// controlPlan starts, stops, restarts or reloads one unit. nginx is checked first: a restart or reload with a broken
// configuration would leave every website down.
func controlPlan(key, u, action string) installer.Plan {
	var acts []installer.Action
	if u == "nginx.service" {
		acts = append(acts, installer.Run("nginx", "-t"))
		if action == services.Reload {
			acts = append(acts, installer.ReloadNginx())
			return installer.Plan{{Name: action + " " + key, Actions: acts}}
		}
	}
	acts = append(acts, installer.Run("systemctl", action, u))
	return installer.Plan{{Name: action + " " + key, Actions: acts}}
}

// alreadyInstalled says whether key is on the machine. The database browser is not counted: installing it again brings it up to the
// version of this program.
func alreadyInstalled(key string) bool {
	kind, v, versioned := services.InstallVersion(key)
	switch {
	case key == "mariadb" || key == "mysql":
		return unitOf("database") != ""
	case versioned && kind == services.GroupPHP:
		return unitOf(key) != ""
	case versioned && kind == services.GroupNode:
		return slices.Contains(site.InstalledNode(unit.NodeRoot), v)
	}
	return false
}

// hostProfile sizes the settings of what is installed to this machine, the way a first install does.
func hostProfile() tune.Profile {
	ram, _ := readMeminfo()
	if ram <= 0 {
		ram = 1024
	}
	return tune.New(tune.Input{RAMMB: ram, CPUs: runtime.NumCPU()})
}

// installPlan returns the plan that adds one thing to a machine that is running: the same step a first install would run for it.
func installPlan(key, series string) (installer.Plan, error) {
	t, err := thisTarget()
	if err != nil {
		return nil, err
	}
	if kind, v, ok := services.InstallVersion(key); ok {
		if kind == services.GroupNode {
			return installer.NodePlan(v)
		}
		if db := unitOf("database"); v < "7.4" && (db == "mysql.service" || db == "mysqld.service") {
			return nil, fmt.Errorf("PHP %s cannot sign in to MySQL 8 unless the server was set up for it when it was installed", v)
		}
		return installer.PHPPlan(installer.Options{Target: t, Profile: hostProfile(), PHPVersions: []string{v}})
	}
	switch key {
	case "mariadb", "mysql":
		return installer.DatabasePlan(installer.Options{Target: t, Profile: hostProfile(), Engine: key, DBSeries: series, PHPVersions: site.InstalledPHP()})
	case services.PhpMyAdmin:
		l := lockedPorts()
		if len(l.Panel) == 0 {
			return nil, fmt.Errorf("the database browser needs the web panel, which is not installed on this machine")
		}
		engine := ""
		switch unitOf("database") {
		case "mariadb.service":
			engine = "mariadb"
		case "":
			return nil, fmt.Errorf("the database browser needs a database server, and there is none on this machine")
		default:
			engine = "mysql"
		}
		return installer.PMAPlan(installer.Options{Target: t, Engine: engine, PanelPort: l.Panel[0], PHPVersions: site.InstalledPHP()})
	}
	return nil, fmt.Errorf("the panel cannot install %q", key)
}

// runServices lists what the panel manages, does start/stop/restart/reload on a service, and installs what can be added after the first
// install. The keys come from a fixed list (internal/services).
func runServices(args []string) {
	if len(args) == 0 {
		servicesUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("services "+sub, flag.ExitOnError)
	asJSON := fs.Bool("json", false, "status: print JSON")
	key := fs.String("key", "", "the service")
	series := fs.String("series", "", "install of mariadb or mysql: the version, such as 11.8 or 8.4; empty means the default")
	action := fs.String("action", "", "control: start, stop, restart or reload")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])

	switch sub {
	case "status":
		items := services.Status(servicesProbe())
		if *asJSON {
			printJSON(items)
			return
		}
		for _, it := range items {
			fmt.Printf("%-12s installed=%v state=%s\n", it.Key, it.Installed, it.State)
		}
	case "control":
		fail(services.ValidAction(*key, *action))
		u := unitOf(*key)
		if u == "" {
			fail(fmt.Errorf("%s is not installed on this machine", *key))
		}
		if !*apply {
			fmt.Printf("# checked only. Would %s %s.\n", *action, u)
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(controlPlan(*key, u, *action)))
		fmt.Printf("%s: %s done\n", u, *action)
	case "install":
		fail(services.ValidInstall(*key))
		if alreadyInstalled(*key) {
			fail(fmt.Errorf("%s is already installed", *key))
		}
		if *series != "" {
			fail(services.ValidSeries(*series))
			if *key != "mariadb" && *key != "mysql" {
				fail(fmt.Errorf("-series applies only to mariadb and mysql"))
			}
		}
		plan, err := installPlan(*key, *series)
		fail(err)
		if !*apply {
			fmt.Print(plan)
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(plan))
		if *key == services.PhpMyAdmin { // a panel that has a name now also serves the browser under /pma/ of that name
			st := openStore(defaultDB)
			err := syncFront(st)
			st.Close()
			fail(err)
		}
		fmt.Printf("%s is installed\n", *key)
	default:
		servicesUsage()
	}
}
