package main

import (
	"flag"
	"fmt"
	"os"

	"mpn/internal/certs"
	"mpn/internal/provision"
	"mpn/internal/site"
)

// siteDomain changes the domain of a site and its www choice. A site has one domain.
//
// A certificate is made for names, so a new domain is a new site as far as SSL goes: when the domain changes, the certificate
// and its key are removed and the site is served over HTTP until a person chooses an SSL source again on the SSL tab. A change
// of the www choice alone keeps the certificate as long as it still covers every name (a wildcard may).
func siteDomain(args []string) {
	fs := flag.NewFlagSet("site domain", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	domain := fs.String("domain", "", "the domain of the site; default: the one it has")
	www := fs.String("www", "", "\"\" for the domain only, \"bare\" for example.com with www redirecting to it, \"www\" for www.example.com with the bare name redirecting to it")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only describe what would happen")
	_ = fs.Parse(args)
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	st := openStore(*db)
	defer st.Close()
	old, err := st.GetSite(*name)
	fail(err)
	cur := old
	if given["www"] {
		if !site.ValidWWW(*www) {
			fail(fmt.Errorf("-www %q: use \"\", \"bare\" or \"www\"", *www))
		}
		cur.WWW = *www
	}
	d := old.Primary()
	if given["domain"] {
		d = *domain
	}
	if !site.ValidDomain(d) {
		fail(fmt.Errorf("-domain %q is not a lower-case host name such as example.com", d))
	}
	cur.Hostnames = site.NamesFor(d, cur.WWW)
	fail(cur.Validate())

	// A name that another site holds is refused before anything is written.
	all, err := st.ListSites()
	fail(err)
	for _, o := range all {
		if o.Name == cur.Name {
			continue
		}
		for _, h := range o.Hostnames {
			for _, n := range cur.Hostnames {
				if h == n {
					fail(fmt.Errorf("%s is already the domain of site %q", n, o.Name))
				}
			}
		}
	}

	dropSSL := false
	if old.TLS {
		if site.BareDomain(old.Primary()) != site.BareDomain(cur.Primary()) {
			dropSSL = true
		} else if pem, err := os.ReadFile(provision.CertDir(old.Name) + "/fullchain.pem"); err != nil || !certs.Covers(pem, cur.Hostnames) {
			dropSSL = true
		}
	}
	if !*apply {
		fmt.Printf("site %q: %v (www %q)\n", cur.Name, cur.Hostnames, cur.WWW)
		if dropSSL {
			fmt.Println("the certificate would be removed: the site goes back to HTTP until SSL is chosen again")
		}
		fmt.Println("# dry run: nothing was changed. Add -apply.")
		return
	}
	requireLinuxRoot()

	if dropSSL {
		cur.TLS, cur.SSLMode = false, site.SSLNone
	}
	plan, err := provision.VhostPlan(cur, "")
	fail(err)
	fail(runPlanQuiet(plan))
	fail(st.SetDomain(cur))
	if dropSSL {
		fail(resetSSL(st, cur))
		fmt.Printf("the certificate of site %q was removed; choose an SSL source again\n", cur.Name)
	}
	fmt.Printf("domain of site %q is now %s\n", cur.Name, cur.Primary())
}
