package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"mpn/internal/installer"
	"mpn/internal/osinfo"
	"mpn/internal/tune"
)

// runPlan prints what the installer would do on the given host. It changes nothing.
func runPlan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	osr := fs.String("os-release", "/etc/os-release", "path to an os-release file")
	ram := fs.Int("ram", 1024, "total RAM in MB")
	cpus := fs.Int("cpus", 1, "CPU count")
	swap := fs.Int("swap", 0, "current swap in MB")
	engine := fs.String("engine", "mariadb", "mariadb, mysql or none")
	series := fs.String("series", "", "database series, such as 11.8 for MariaDB or 8.4 for MySQL; empty means the engine's default")
	phpList := fs.String("php", "", "comma-separated PHP versions to install, such as 7.4,8.4")
	nodeList := fs.String("node", "", "comma-separated Node.js major versions to install, such as 22,24")
	panel := fs.Bool("panel", false, "also install the web panel (HTTPS on -panel-port, behind nginx)")
	panelPort := fs.Int("panel-port", 2610, "public HTTPS port of the web panel")
	var panelHosts hostList
	fs.Var(&panelHosts, "panel-host", "name the panel is reached by, repeat for several (IP addresses are always accepted)")
	pma := fs.Bool("pma", false, "also install the database browser (phpMyAdmin) on the port after the panel's; needs -panel and a database engine")
	apply := fs.Bool("apply", false, "apply the plan to this machine (Linux, root); RAM, CPUs and swap are detected unless given")
	_ = fs.Parse(args)

	if *apply {
		detectHost(fs, ram, cpus, swap)
	}
	raw, err := os.ReadFile(*osr)
	fail(err)
	t, err := osinfo.Parse(string(raw))
	fail(err)
	eng := *engine
	if eng == "none" {
		eng = ""
	}
	plan, err := installer.Build(installer.Options{
		Target: t, Profile: tune.New(tune.Input{RAMMB: *ram, CPUs: *cpus}),
		CurrentSwapMB: *swap, Engine: eng, DBSeries: *series,
		PHPVersions: splitList(*phpList), NodeVersions: splitList(*nodeList), SelfPath: selfPath(),
		Panel: *panel, PanelPort: *panelPort, PanelHosts: panelHosts, PMA: *pma,
	})
	fail(err)
	if *apply {
		runApply(plan)
		return
	}
	fmt.Print(plan)
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func fail(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
