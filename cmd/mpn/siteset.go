package main

import (
	"flag"
	"fmt"
	"maps"
	"slices"
	"strings"

	"mpn/internal/provision"
	"mpn/internal/site"
	"mpn/internal/tune"
	"mpn/internal/unit"
)

// iniList collects "-ini name=value" flags.
type iniList []string

func (l *iniList) String() string     { return strings.Join(*l, ",") }
func (l *iniList) Set(v string) error { *l = append(*l, v); return nil }

// siteSet changes the settings of a site that already exists: the web root below htdocs, the PHP version and settings,
// the version, port, start script and memory of a node app, and the address a proxy forwards to. Only the flags that are
// given change anything, and a flag that does not fit the kind of the site is an error rather than ignored.
//
// With -apply the files of the site are made again, and the database is updated only after that worked, so the stored
// settings always describe what is running. If making the files fails, the executor puts back the files it had written.
func siteSet(args []string) {
	fs := flag.NewFlagSet("site set", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	rootSub := fs.String("root-sub", "", "web root below htdocs, for example public; empty is htdocs itself")
	php := fs.String("php", "", "PHP version, php sites only")
	var ini iniList
	fs.Var(&ini, "ini", "PHP setting as name=value, repeat for several, php sites only")
	iniReset := fs.Bool("ini-reset", false, "forget the PHP settings that were set before, php sites only; with -ini the new ones follow")
	node := fs.String("node", "", "Node.js major version, node sites only")
	python := fs.String("python", "", "Python 3 minor version such as 3.12, python sites only")
	entry := fs.String("entry", "", "start script relative to the root (python also accepts \"-m module\"), node and python sites only")
	port := fs.Int("port", 0, "port of the app, node and python sites only")
	mem := fs.Int("mem", 0, "memory the app may use in MB, 0 for the default, node and python sites only")
	upstream := fs.String("upstream", "", "http(s) URL, proxy sites only")
	apply := fs.Bool("apply", false, "make the changes on this machine (Linux, root); otherwise the plan is only printed")
	r := addRenderFlags(fs)
	_ = fs.Parse(args)
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	if *apply {
		var cpus, swap int
		detectHost(fs, r.ram, &cpus, &swap)
	}
	st := openStore(*db)
	defer st.Close()
	old, err := st.GetSite(*name)
	fail(err)
	cur := old
	cur.PHPIni = maps.Clone(old.PHPIni)

	onlyFor := func(flagName string, kinds ...site.Kind) {
		if given[flagName] && !slices.Contains(kinds, cur.Kind) {
			names := make([]string, len(kinds))
			for i, k := range kinds {
				names[i] = string(k)
			}
			fail(fmt.Errorf("-%s applies to %s sites only, and site %q is a %s site", flagName, strings.Join(names, " and "), cur.Name, cur.Kind))
		}
	}
	for _, f := range []string{"php", "ini", "ini-reset"} {
		onlyFor(f, site.PHP)
	}
	onlyFor("node", site.Node)
	onlyFor("python", site.Python)
	for _, f := range []string{"entry", "port", "mem"} {
		onlyFor(f, site.Node, site.Python)
	}
	onlyFor("upstream", site.Proxy)

	if given["root-sub"] {
		if cur.Kind == site.Proxy {
			fail(fmt.Errorf("a proxy site has no web root"))
		}
		if !site.ValidSubdir(*rootSub) {
			fail(fmt.Errorf("-root-sub %q: use folder names of letters, digits, '.', '_' and '-', separated by '/', none starting with a dot", *rootSub))
		}
		cur.Root = site.RootFor(cur.User, *rootSub)
	}
	if given["php"] {
		cur.PHPVersion = *php
	}
	if *iniReset {
		cur.PHPIni = nil
	}
	for _, kv := range ini {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			fail(fmt.Errorf("-ini %q: write it as name=value", kv))
		}
		if cur.PHPIni == nil {
			cur.PHPIni = map[string]string{}
		}
		cur.PHPIni[k] = v
	}
	if given["node"] {
		cur.NodeVersion = *node
	}
	if given["python"] {
		cur.PyVersion = *python
	}
	if given["entry"] {
		cur.Entry = *entry
	}
	if given["port"] {
		cur.NodePort = *port
		if f := mpnPortHolder(*port); f != "" {
			fail(fmt.Errorf("port %d is used by the panel (%s); an app needs another port", *port, f))
		}
		if other, err := portHolder(st, cur.Name, *port); err != nil {
			fail(err)
		} else if other != "" {
			fail(fmt.Errorf("port %d is already used by site %q", *port, other))
		}
	}
	if given["mem"] {
		cur.MemMB = *mem
	}
	if given["upstream"] {
		cur.Upstream = *upstream
	}
	fail(cur.Validate())
	if cur.Kind == site.PHP && cur.PHPVersion != old.PHPVersion && !slices.Contains(site.InstalledPHP(), cur.PHPVersion) {
		fail(fmt.Errorf("php %s is not installed on this machine; install it first with: mpn plan -php %s -apply", cur.PHPVersion, cur.PHPVersion))
	}
	if cur.Kind == site.Node && cur.NodeVersion != old.NodeVersion && !slices.Contains(site.InstalledNode(unit.NodeRoot), cur.NodeVersion) {
		fail(fmt.Errorf("node %s is not installed on this machine; install it first with: mpn plan -node %s -apply", cur.NodeVersion, cur.NodeVersion))
	}
	if cur.Kind == site.Python && cur.PyVersion != old.PyVersion && !slices.Contains(site.InstalledPython("/usr/bin"), cur.PyVersion) {
		fail(fmt.Errorf("python %s is not installed on this machine; install the package python%s (and python%s-venv) first", cur.PyVersion, cur.PyVersion, cur.PyVersion))
	}

	plan, err := provision.ChangePlan(old, cur, provision.Options{
		Profile: tune.New(tune.Input{RAMMB: *r.ram, CPUs: 1}),
		TLS:     *r.tls, CertDir: *r.certDir, NodeMemMB: *r.nodeMem,
	})
	fail(err)
	if !*apply {
		fmt.Print(plan)
		fmt.Println("# dry run: nothing was changed. With -apply the files are made and then the settings are stored.")
		return
	}
	runApply(plan)
	fail(st.UpdateSite(cur))
	fmt.Printf("settings of site %q stored\n", cur.Name)
}

// portHolder returns the name of another site that uses the port, or "".
func portHolder(st interface {
	ListSites() ([]site.Site, error)
}, self string, port int) (string, error) {
	all, err := st.ListSites()
	if err != nil {
		return "", err
	}
	for _, s := range all {
		if s.Name != self && (s.Kind == site.Node || s.Kind == site.Python) && s.NodePort == port {
			return s.Name, nil
		}
	}
	return "", nil
}
