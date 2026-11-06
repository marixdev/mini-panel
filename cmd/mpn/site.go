package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"mpn/internal/site"
	"mpn/internal/store"
	"mpn/internal/tune"
	"mpn/internal/unit"
	"mpn/internal/vhost"
)

const (
	defaultDB  = "/var/lib/mpn/mpn.db"
	nodePortLo = 20000
	nodePortHi = 29999
)

type hostList []string

func (h *hostList) String() string     { return strings.Join(*h, ",") }
func (h *hostList) Set(v string) error { *h = append(*h, v); return nil }

// renderFlags are shared by the commands that print the files a site needs.
type renderFlags struct {
	tls     *bool
	certDir *string
	ram     *int
	nodeMem *int
}

func addRenderFlags(fs *flag.FlagSet) renderFlags {
	return renderFlags{
		fs.Bool("tls", false, "render the HTTPS server block"),
		fs.String("certdir", "", "certificate directory for -tls (default /etc/mpn/certs/<name>)"),
		fs.Int("ram", 1024, "host RAM in MB, sizes the PHP pool"),
		fs.Int("nodemem", 256, "MemoryMax in MB for node apps, 0 for no cap"),
	}
}

func runSite(args []string) {
	if len(args) == 0 {
		siteUsage()
	}
	switch args[0] {
	case "add":
		siteAdd(args[1:])
	case "list":
		siteList(args[1:])
	case "show":
		siteShow(args[1:])
	case "delete":
		siteDelete(args[1:])
	case "apply":
		siteApply(args[1:])
	case "set":
		siteSet(args[1:])
	case "info":
		siteInfoCmd(args[1:])
	case "remove":
		siteRemove(args[1:])
	case "access":
		siteAccess(args[1:])
	case "domain":
		siteDomain(args[1:])
	case "vhost":
		siteVhost(args[1:])
	case "ssl":
		siteSSL(args[1:])
	case "ssl-custom":
		siteSSLCustom(args[1:])
	case "ssl-csr":
		siteCSR(args[1:])
	case "ssl-off":
		siteSSLOff(args[1:])
	case "ssl-redirect":
		siteSSLRedirect(args[1:])
	case "ssl-info":
		siteSSLInfo(args[1:])
	case "ssl-renew":
		siteRenew(args[1:])
	default:
		siteUsage()
	}
}

func siteUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn site <add|list|show|delete|apply|set|info|domain|vhost|remove|access|ssl|ssl-renew> [flags]   (add -h for the flags)")
	os.Exit(2)
}

func openStore(path string) *store.Store {
	fail(os.MkdirAll(filepath.Dir(path), 0o755))
	st, err := store.Open(path)
	fail(err)
	return st
}

func siteAdd(args []string) {
	fs := flag.NewFlagSet("site add", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	kind := fs.String("kind", "php", "static, php, node, python or proxy")
	var hosts hostList
	fs.Var(&hosts, "host", "hostname, repeat for aliases (the first is primary)")
	var s site.Site
	fs.StringVar(&s.Name, "name", "", "site name")
	fs.StringVar(&s.User, "user", "", "system user (default: the site name)")
	fs.StringVar(&s.Root, "root", "", "document root (default: /home/<user>/htdocs)")
	fs.StringVar(&s.PHPVersion, "php", "8.4", "PHP version, php sites only")
	fs.StringVar(&s.NodeVersion, "node", "22", "Node.js major version, node sites only")
	fs.StringVar(&s.PyVersion, "python", "", "Python 3 minor version such as 3.12, python sites only (default: the system's python3)")
	fs.StringVar(&s.Entry, "entry", "", "start script relative to the root (default server.js for node, app.py for python)")
	fs.IntVar(&s.NodePort, "port", 0, "node or python app port, 0 picks a free one")
	fs.StringVar(&s.Upstream, "upstream", "", "http(s) URL, proxy sites only")
	r := addRenderFlags(fs)
	_ = fs.Parse(args)

	s.Kind, s.Hostnames = site.Kind(*kind), hosts
	if s.User == "" {
		s.User = s.Name
	}
	if s.Root == "" {
		s.Root = "/home/" + s.User + "/htdocs"
	}
	st := openStore(*db)
	defer st.Close()
	switch s.Kind {
	case site.Node:
		if s.Entry == "" {
			s.Entry = "server.js"
		}
	case site.Python:
		if s.Entry == "" {
			s.Entry = "app.py"
		}
		if s.PyVersion == "" {
			s.PyVersion = site.SystemPython()
		}
	}
	if (s.Kind == site.Node || s.Kind == site.Python) && s.NodePort == 0 {
		p, err := st.AllocNodePort(nodePortLo, nodePortHi)
		fail(err)
		s.NodePort = p
	}
	fail(st.AddSite(s))
	saved, err := st.GetSite(s.Name)
	fail(err)
	fmt.Printf("# site %q saved in %s (nothing was written to the system)\n\n", saved.Name, *db)
	fail(printArtifacts(saved, r))
}

// expiryOf is when the certificate of a site with HTTPS ends, or 0.
func expiryOf(s site.Site) int64 {
	if !s.TLS {
		return 0
	}
	return certExpiry(s.Name)
}

func siteList(args []string) {
	fs := flag.NewFlagSet("site list", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	asJSON := fs.Bool("json", false, "print JSON instead of a table")
	rows := []siteRow{}
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	list, err := st.ListSites()
	fail(err)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tKIND\tUSER\tHOSTNAMES\tDETAIL")
	for _, s := range list {
		detail := ""
		switch s.Kind {
		case site.PHP:
			detail = "php " + s.PHPVersion
		case site.Node:
			detail = fmt.Sprintf("node %s on port %d", s.NodeVersion, s.NodePort)
		case site.Python:
			detail = fmt.Sprintf("python %s on port %d", s.PyVersion, s.NodePort)
		case site.Proxy:
			detail = s.Upstream
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Name, s.Kind, s.User, strings.Join(s.Hostnames, ","), detail)
		rows = append(rows, siteRow{Name: s.Name, Kind: string(s.Kind), User: s.User, Hostnames: append([]string{}, s.Hostnames...), Detail: detail, TLS: s.TLS, Root: s.Root, WWW: s.WWW, AllowHTTP: s.AllowHTTP, CertExpiry: expiryOf(s), CustomVhost: s.CustomVhost != ""})
	}
	if *asJSON {
		printJSON(rows)
		return
	}
	fail(w.Flush())
}

func siteShow(args []string) {
	fs := flag.NewFlagSet("site show", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	r := addRenderFlags(fs)
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	fail(printArtifacts(s, r))
}

func siteDelete(args []string) {
	fs := flag.NewFlagSet("site delete", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	fail(st.DeleteSite(*name))
	fmt.Printf("site %q removed from the database (no files, users or services were touched)\n", *name)
}

// printArtifacts prints every file the site needs, each under a header with its target path.
func printArtifacts(s site.Site, r renderFlags) error {
	cert := *r.certDir
	if cert == "" {
		cert = "/etc/mpn/certs/" + s.Name
	}
	conf, err := vhost.Render(s, vhost.Options{TLS: *r.tls || s.TLS, CertDir: cert})
	if err != nil {
		return err
	}
	fmt.Printf("# --- /etc/nginx/sites-enabled/%s.conf\n%s\n", s.Name, conf)
	switch s.Kind {
	case site.PHP:
		p := tune.New(tune.Input{RAMMB: *r.ram, CPUs: 1})
		fmt.Printf("# --- /etc/php/%s/fpm/pool.d/%s.conf\n%s", s.PHPVersion, s.Name, p.PHPPool(s.Name, s.User, "/run/php/"+s.Name+".sock"))
	case site.Node:
		u, err := unit.Node(s, *r.nodeMem)
		if err != nil {
			return err
		}
		fmt.Printf("# --- /etc/systemd/system/%s\n%s", unit.Name(s), u)
	case site.Python:
		u, err := unit.Python(s, *r.nodeMem)
		if err != nil {
			return err
		}
		fmt.Printf("# --- /etc/systemd/system/%s\n%s", unit.Name(s), u)
	}
	return nil
}
