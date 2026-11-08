package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"mpn/internal/acme"
	"mpn/internal/certs"
	"mpn/internal/dnscheck"
	"mpn/internal/installer"
	"mpn/internal/provision"
	"mpn/internal/site"
)

// A site has one certificate, from one source: Let's Encrypt (mpn renews it) or one the person brought (they replace it).
// The commands here choose and change that source. Each writes the files first and the database after, so what the database
// says is what runs, and a failing step leaves the old files in place (the executor puts them back).

func pendingKey(name string) string { return provision.CertDir(name) + "/pending.key" }
func pendingCSR(name string) string { return provision.CertDir(name) + "/pending.csr" }

// siteSSL gets a Let's Encrypt certificate for the names of a site and switches the site to SSL. The names are checked first:
// a name that does not lead to this machine would fail at the authority, and the authority allows only a few failures an hour.
func siteSSL(args []string) {
	fs := flag.NewFlagSet("site ssl", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	email := fs.String("email", "", "contact address for Let's Encrypt (optional)")
	staging := fs.Bool("staging", false, "use the Let's Encrypt test CA; the certificate is saved but not enabled")
	apply := fs.Bool("apply", false, "contact the CA and change this machine (Linux, root); otherwise only describe what would happen")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	refuseCustomVhost(s, "SSL")
	cfg := acmeConfig(*email, *staging)
	if !*apply {
		fmt.Printf("would check that %v lead to this server, request a certificate from %s,\nsave it in %s and %s\n", s.Hostnames, cfg.Directory, provision.CertDir(s.Name), map[bool]string{true: "leave SSL off (staging)", false: "switch the site to SSL"}[*staging])
		return
	}
	requireLinuxRoot()
	if !*staging {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		if f := dnscheck.Check(ctx, realDNS(), s.Hostnames); f != nil {
			cancel()
			fail(f)
		}
		cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	chain, key, err := acme.Issue(ctx, cfg, s.Hostnames)
	fail(err)
	s.SSLMode = site.SSLLetsEncrypt
	s.TLS = !*staging
	plan, err := provision.TLSPlan(s, chain, key, !*staging)
	fail(err)
	runApply(plan)
	if !*staging {
		fail(st.SetSSL(s.Name, site.SSLLetsEncrypt))
		fmt.Printf("site %q now serves SSL (Let's Encrypt)\n", s.Name)
	}
}

// customInput is what the web panel sends on standard input: the text of the three boxes. It never travels in an argument.
type customInput struct {
	Key            string `json:"key"`   // empty: use the key made with "site ssl-csr"
	Cert           string `json:"cert"`  // the certificate, or the whole bundle
	Chain          string `json:"chain"` // the intermediates, if they were pasted apart
	AllowUntrusted bool   `json:"allow_untrusted"`
}

const untrustedMarker = "ssl-untrusted"

// siteSSLCustom installs a certificate the person brought. The key, the certificate and the names are checked before a file is
// written; the private key stays in a file only root can read.
func siteSSLCustom(args []string) {
	fs := flag.NewFlagSet("site ssl-custom", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	stdin := fs.Bool("stdin", true, "read {key, cert, chain, allow_untrusted} as JSON on standard input")
	apply := fs.Bool("apply", false, "install it on this machine (Linux, root); otherwise only check it")
	_ = fs.Parse(args)
	_ = stdin
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	refuseCustomVhost(s, "SSL")

	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
	fail(err)
	var in customInput
	if err := json.Unmarshal(raw, &in); err != nil {
		fail(fmt.Errorf("the input is not the JSON that mpn expects: %w", err))
	}
	key := []byte(in.Key)
	if in.Key == "" {
		if key, err = os.ReadFile(pendingKey(s.Name)); err != nil {
			fail(errors.New("no private key was pasted, and no request (CSR) was made on this panel for this site"))
		}
	}
	res, err := certs.Check(key, []byte(in.Cert), []byte(in.Chain), s.Hostnames, time.Now(), nil)
	switch {
	case errors.Is(err, certs.ErrUntrusted) && in.AllowUntrusted:
	case errors.Is(err, certs.ErrUntrusted):
		fail(fmt.Errorf("%s: the certificate does not chain to an authority that browsers trust; paste the intermediate certificates (the CA bundle) too. "+
			"If it is an origin certificate for a proxy such as Cloudflare, confirm that to install it anyway", untrustedMarker))
	default:
		fail(err)
	}
	fmt.Printf("certificate for %v from %s, valid until %s\n", res.Names, res.Issuer, res.NotAfter.Format("2006-01-02"))
	if !*apply {
		fmt.Println("# checked only. Add -apply to install it.")
		return
	}
	requireLinuxRoot()
	s.SSLMode, s.TLS = site.SSLCustom, true
	plan, err := provision.TLSPlan(s, res.Fullchain, res.Key, true)
	fail(err)
	plan = append(plan, installer.Step{Name: "pending request", Actions: []installer.Action{
		installer.Run("rm", "-f", pendingKey(s.Name), pendingCSR(s.Name)),
	}})
	runApply(plan)
	fail(st.SetSSL(s.Name, site.SSLCustom))
	fmt.Printf("site %q now serves SSL (your certificate)\n", s.Name)
}

// siteCSR makes a private key that stays on this machine and prints the request to give to the authority that sells the certificate.
func siteCSR(args []string) {
	fs := flag.NewFlagSet("site ssl-csr", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	algo := fs.String("algo", "ecdsa", "key type: ecdsa (P-256) or rsa (2048 bits)")
	apply := fs.Bool("apply", false, "save the key on this machine (Linux, root)")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	keyPEM, csrPEM, err := certs.NewCSR(*algo, s.Hostnames)
	fail(err)
	if !*apply {
		fmt.Printf("would make a %s key and a request for %v\n", *algo, s.Hostnames)
		return
	}
	requireLinuxRoot()
	plan := installer.Plan{{Name: "certificate request", Actions: []installer.Action{
		installer.Run("install", "-d", "-m", "0700", "-o", "root", "-g", "root", provision.CertDir(s.Name)),
		installer.WriteFile(pendingKey(s.Name), string(keyPEM), 0o600),
		installer.WriteFile(pendingCSR(s.Name), string(csrPEM), 0o644),
	}}}
	fail(runPlanQuiet(plan))
	fmt.Print(string(csrPEM))
}

// siteSSLOff takes the certificate away and serves the site over HTTP again.
func siteSSLOff(args []string) {
	fs := flag.NewFlagSet("site ssl-off", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	apply := fs.Bool("apply", false, "change this machine (Linux, root)")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	refuseCustomVhost(s, "removing SSL")
	if !*apply {
		fmt.Println("would serve the site over HTTP and delete its certificate files")
		return
	}
	requireLinuxRoot()
	fail(resetSSL(st, s))
	fmt.Printf("site %q now serves HTTP only\n", s.Name)
}

// resetSSL serves the site over HTTP and deletes its certificate and key. The vhost goes first, so nginx does not look for
// files that are gone.
func resetSSL(st interface{ SetSSL(string, string) error }, s site.Site) error {
	s.TLS, s.SSLMode = false, site.SSLNone
	plan, err := provision.VhostPlan(s, "")
	if err != nil {
		return err
	}
	plan = append(plan, installer.Step{Name: "certificate files", Actions: []installer.Action{installer.Run("rm", "-rf", provision.CertDir(s.Name))}})
	if err := runPlanQuiet(plan); err != nil {
		return err
	}
	return st.SetSSL(s.Name, site.SSLNone)
}

// siteSSLRedirect chooses whether visitors of the plain HTTP address are sent to SSL.
func siteSSLRedirect(args []string) {
	fs := flag.NewFlagSet("site ssl-redirect", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	redirect := fs.Bool("redirect", true, "send visitors of http:// to SSL; false keeps serving http:// as well")
	apply := fs.Bool("apply", false, "change this machine (Linux, root)")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	refuseCustomVhost(s, "the redirect")
	s.AllowHTTP = !*redirect
	if !*apply {
		fmt.Printf("would set: send http:// visitors to SSL = %v\n", *redirect)
		return
	}
	requireLinuxRoot()
	plan, err := provision.VhostPlan(s, "")
	fail(err)
	fail(runPlanQuiet(plan))
	fail(st.SetDomain(s))
	fmt.Printf("site %q: http:// visitors are %s\n", s.Name, map[bool]string{true: "sent to SSL", false: "served as they are"}[*redirect])
}

// siteSpeed turns the nginx pagespeed module (minify HTML/CSS/JS, optimize images) on or off for a site.
func siteSpeed(args []string) {
	fs := flag.NewFlagSet("site speed", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	on := fs.Bool("on", true, "compress text and cache static files in the browser")
	apply := fs.Bool("apply", false, "change this machine (Linux, root)")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	refuseCustomVhost(s, "the speed settings")
	s.Speed = *on
	if !*apply {
		fmt.Printf("would set: speed = %v\n", *on)
		return
	}
	requireLinuxRoot()
	plan, err := provision.VhostPlan(s, "")
	fail(err)
	fail(runPlanQuiet(plan))
	fail(st.SetDomain(s))
	fmt.Printf("site %q: speed is %s\n", s.Name, map[bool]string{true: "on", false: "off"}[*on])
}

// certInfo is the answer of "site ssl-info": what the SSL tab shows.
type certInfo struct {
	Mode       string   `json:"mode"`
	Issuer     string   `json:"issuer,omitempty"`
	Names      []string `json:"names,omitempty"`
	NotBefore  int64    `json:"not_before,omitempty"`
	NotAfter   int64    `json:"not_after,omitempty"`
	Covers     bool     `json:"covers"`                // the certificate covers every name of the site
	PendingCSR string   `json:"pending_csr,omitempty"` // a request made here and not answered yet
	Trusted    bool     `json:"trusted"`
}

func siteSSLInfo(args []string) {
	fs := flag.NewFlagSet("site ssl-info", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	out := certInfo{Mode: s.SSLMode}
	if b, err := os.ReadFile(pendingCSR(s.Name)); err == nil {
		out.PendingCSR = string(b)
	}
	if b, err := os.ReadFile(provision.CertDir(s.Name) + "/fullchain.pem"); err == nil && s.TLS {
		if info, err := certs.Describe(b); err == nil {
			out.Issuer, out.Names, out.NotBefore, out.NotAfter = info.Issuer, info.Names, info.NotBefore.Unix(), info.NotAfter.Unix()
			out.Covers = certs.Covers(b, s.Hostnames)
		}
		out.Trusted = chainTrusted(b)
	}
	printJSON(out)
}

// chainTrusted reports whether the installed certificate chains to an authority in the system's store.
func chainTrusted(fullchain []byte) bool {
	all, err := parseAll(fullchain)
	if err != nil || len(all) == 0 {
		return false
	}
	inter := x509.NewCertPool()
	for _, c := range all[1:] {
		inter.AddCert(c)
	}
	_, err = all[0].Verify(x509.VerifyOptions{Intermediates: inter})
	return err == nil
}
