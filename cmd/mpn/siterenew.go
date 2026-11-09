package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"mpn/internal/acme"
	"mpn/internal/executor"
	"mpn/internal/provision"
	"mpn/internal/site"
)

func acmeConfig(email string, staging bool) acme.Config {
	if staging {
		return acme.Config{Directory: acme.Staging, Email: email, AccountKey: "/etc/mpn/acme/account-staging.key", WebRoot: "/var/lib/mpn/acme"}
	}
	return acme.Config{Directory: acme.Production, Email: email, AccountKey: "/etc/mpn/acme/account.key", WebRoot: "/var/lib/mpn/acme"}
}

// siteRenew renews the certificate of every HTTPS site that expires within -within days.
// It exits with status 1 if any site could not be checked or renewed, so a timer unit shows the failure.
func siteRenew(args []string) {
	fs := flag.NewFlagSet("site ssl-renew", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	within := fs.Int("within", 30, "renew certificates that expire within this many days")
	email := fs.String("email", "", "contact address for Let's Encrypt (optional)")
	apply := fs.Bool("apply", false, "contact the CA and replace certificates (Linux, root); otherwise only report")
	_ = fs.Parse(args)
	if *apply {
		requireLinuxRoot()
	}
	st := openStore(*db)
	defer st.Close()
	list, err := st.ListSites()
	fail(err)
	failed := 0
	for _, s := range list {
		if !s.TLS || s.SSLMode != site.SSLLetsEncrypt {
			continue // a certificate the person brought is theirs to replace
		}
		pemBytes, err := os.ReadFile(provision.CertDir(s.Name) + "/fullchain.pem")
		if err == nil {
			var days int
			if days, err = acme.DaysLeft(pemBytes, time.Now()); err == nil {
				switch {
				case days > *within:
					fmt.Printf("%s: %d days left, nothing to do\n", s.Name, days)
					continue
				case !*apply:
					fmt.Printf("%s: %d days left, would renew (run with -apply)\n", s.Name, days)
					continue
				}
				fmt.Printf("%s: %d days left, renewing\n", s.Name, days)
				err = renewOne(acmeConfig(*email, false), s)
			}
		}
		if err != nil {
			fmt.Printf("%s: FAILED: %v\n", s.Name, err)
			failed++
			continue
		}
		fmt.Printf("%s: renewed\n", s.Name)
	}
	failed += renewPanelCert(st, *within, *apply, *email)
	failed += renewMailCert(st, *within, *apply, *email)
	if failed > 0 {
		os.Exit(1)
	}
}

// renewOne gets a new certificate and installs it. The executor restores the old files if nginx rejects the result.
func renewOne(cfg acme.Config, s site.Site) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	chain, key, err := acme.Issue(ctx, cfg, s.Hostnames)
	if err != nil {
		return err
	}
	plan, err := provision.TLSPlan(s, chain, key, true)
	if err != nil {
		return err
	}
	return executor.Apply(plan, &executor.OS{Out: os.Stdout}, os.Stdout)
}
