package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"mpn/internal/acme"
	"mpn/internal/certs"
	"mpn/internal/dnscheck"
	"mpn/internal/installer"
	"mpn/internal/panelssl"
	"mpn/internal/store"
)

func panelUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn panel <ssl-info|ssl|ssl-custom|ssl-off> [flags]   (add -h for the flags)")
	os.Exit(2)
}

// listenPorts finds the TLS ports an nginx file listens on. The panel and its database browser are separate files that read the
// same certificate directory, so one certificate there serves both ports.
func listenPorts(files ...string) []int {
	var out []int
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil {
			for _, m := range listenSSLRe.FindAllStringSubmatch(string(b), -1) {
				n, _ := strconv.Atoi(m[1])
				out = append(out, n)
			}
		}
	}
	return uniqSorted(out)
}

func pmaFiles() []string {
	m, _ := filepath.Glob("/etc/nginx/conf.d/*mpn*pma*.conf")
	return m
}

// panelSSLInfo is the answer of "mpn panel ssl-info".
type panelSSLInfo struct {
	Mode      string   `json:"mode"`
	Domain    string   `json:"domain"`
	Issuer    string   `json:"issuer"`
	Names     []string `json:"names"`
	NotBefore int64    `json:"not_before"`
	NotAfter  int64    `json:"not_after"`
	Covers    bool     `json:"covers"`
	Trusted   bool     `json:"trusted"`
	PanelPort int      `json:"panel_port"`
	PMAPort   int      `json:"pma_port"`
}

func currentPanelSSL(st *store.Store) panelSSLInfo {
	mode, _ := st.Setting(panelssl.SettingMode, panelssl.ModeSelfSigned)
	domain, _ := st.Setting(panelssl.SettingDomain, "")
	out := panelSSLInfo{Mode: mode, Domain: domain}
	if p := listenPorts(panelNginx); len(p) > 0 {
		out.PanelPort = p[0]
	}
	if p := listenPorts(pmaFiles()...); len(p) > 0 {
		out.PMAPort = p[0]
	}
	if b, err := os.ReadFile(panelssl.Dir + "/fullchain.pem"); err == nil {
		if info, err := certs.Describe(b); err == nil {
			out.Issuer, out.Names, out.NotBefore, out.NotAfter = info.Issuer, info.Names, info.NotBefore.Unix(), info.NotAfter.Unix()
			out.Covers = domain != "" && certs.Covers(b, []string{domain})
		}
		out.Trusted = chainTrusted(b)
	}
	return out
}

// installPanelCert writes the certificate, the name the panel answers to and, for a certificate that did not come from Let's
// Encrypt, removes the challenge block. nginx -t runs before the reload and a failure restores the files written here.
func installPanelCert(domain string, chain, key []byte, removeACME bool) error {
	acts := []installer.Action{
		installer.Run("install", "-d", "-m", "0755", "-o", "root", "-g", "root", "/etc/mpn"),
		installer.WriteFile(panelssl.Dir+"/fullchain.pem", string(chain), 0o644),
		installer.WriteFile(panelssl.Dir+"/privkey.pem", string(key), 0o600),
		installer.WriteFile(panelssl.HostsFile, panelssl.HostsText(domain), 0o644),
	}
	if removeACME {
		acts = append(acts, installer.Run("rm", "-f", panelssl.ACMEConf))
	}
	acts = append(acts, installer.Run("nginx", "-t"), installer.ReloadNginx())
	return runPlanQuiet(installer.Plan{{Name: "certificate of the panel", Actions: acts}})
}

// issuePanelCert gets a Let's Encrypt certificate for domain and installs it. The block that answers the challenge on port 80 is
// written first, because the check that the name leads here asks that block for a file. When it was not there before and the
// attempt fails, it is taken away again.
func issuePanelCert(cfg acme.Config, domain string, port int, checkDNS bool) error {
	existed := fileExists(panelssl.ACMEConf)
	conf, err := panelssl.ACMEConfig(domain, port)
	if err != nil {
		return err
	}
	undo := func() {
		if !existed {
			_ = runPlanQuiet(installer.Plan{{Name: "undo", Actions: []installer.Action{installer.Run("rm", "-f", panelssl.ACMEConf), installer.ReloadNginx()}}})
		}
	}
	if err := runPlanQuiet(installer.Plan{{Name: "challenge for the name of the panel", Actions: []installer.Action{
		installer.Run("install", "-d", "-m", "0755", "/var/lib/mpn/acme"),
		installer.WriteFile(panelssl.ACMEConf, conf, 0o644),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}}}); err != nil {
		return err
	}
	if checkDNS {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		f := dnscheck.Check(ctx, realDNS(), []string{domain})
		cancel()
		if f != nil {
			undo()
			return f
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	chain, key, err := acme.Issue(ctx, cfg, []string{domain})
	if err != nil {
		undo()
		return err
	}
	return installPanelCert(domain, chain, key, false)
}

const selfSignedScript = `set -e
d=` + panelssl.Dir + `
mkdir -p "$d"
t=$(mktemp -d "$d/.new.XXXXXX")
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 3650 -subj /CN=mpn-panel -keyout "$t/privkey.pem" -out "$t/fullchain.pem" 2>/dev/null
chmod 600 "$t/privkey.pem"; chmod 644 "$t/fullchain.pem"
mv "$t/privkey.pem" "$d/privkey.pem"; mv "$t/fullchain.pem" "$d/fullchain.pem"
rmdir "$t"`

func printPanelURLs(domain string) {
	info := panelSSLInfo{}
	if p := listenPorts(panelNginx); len(p) > 0 {
		info.PanelPort = p[0]
	}
	if p := listenPorts(pmaFiles()...); len(p) > 0 {
		info.PMAPort = p[0]
	}
	fmt.Printf("panel:       https://%s:%d\n", domain, info.PanelPort)
	if info.PMAPort > 0 {
		fmt.Printf("phpMyAdmin:  https://%s:%d\n", domain, info.PMAPort)
	}
	fmt.Println("Open the new address and sign in again: a session belongs to the address it was made on.")
}

// runPanel manages the certificate of the panel itself. One certificate serves the panel and the database browser, which read the
// same directory.
func runPanel(args []string) {
	if len(args) == 0 {
		panelUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("panel "+sub, flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	domain := fs.String("domain", "", "the name the panel is reached by, such as panel.example.com")
	email := fs.String("email", "", "contact address for Let's Encrypt (optional)")
	staging := fs.Bool("staging", false, "ssl: use the Let's Encrypt test CA (for trials; browsers do not trust the result)")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])
	st := openStore(*db)
	defer st.Close()

	switch sub {
	case "ssl-info":
		printJSON(currentPanelSSL(st))
	case "ssl":
		fail(panelssl.ValidDomain(*domain))
		fail(refuseSiteName(st, *domain))
		ports := listenPorts(panelNginx)
		if len(ports) == 0 {
			fail(errors.New("the panel is not installed on this machine (no " + panelNginx + ")"))
		}
		if !*apply {
			fmt.Printf("would check that %s leads to this server, get a certificate from Let's Encrypt, and use it for the panel (port %d) and the database browser\n", *domain, ports[0])
			return
		}
		requireLinuxRoot()
		fail(issuePanelCert(acmeConfig(*email, *staging), *domain, ports[0], true))
		fail(st.SetSetting(panelssl.SettingMode, panelssl.ModeLetsEncrypt))
		fail(st.SetSetting(panelssl.SettingDomain, *domain))
		fmt.Println("the panel now has a Let's Encrypt certificate; mpn renews it")
		printPanelURLs(*domain)
	case "ssl-custom":
		fail(panelssl.ValidDomain(*domain))
		fail(refuseSiteName(st, *domain))
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
		fail(err)
		var in customInput
		if err := json.Unmarshal(raw, &in); err != nil {
			fail(fmt.Errorf("the input is not the JSON that mpn expects: %w", err))
		}
		if in.Key == "" {
			fail(errors.New("paste the private key of the certificate"))
		}
		res, err := certs.Check([]byte(in.Key), []byte(in.Cert), []byte(in.Chain), []string{*domain}, time.Now(), nil)
		switch {
		case errors.Is(err, certs.ErrUntrusted) && in.AllowUntrusted:
		case errors.Is(err, certs.ErrUntrusted):
			fail(fmt.Errorf("%s: the certificate does not chain to an authority that browsers trust; paste the intermediate certificates (the CA bundle) too", untrustedMarker))
		default:
			fail(err)
		}
		fmt.Printf("certificate for %v from %s, valid until %s\n", res.Names, res.Issuer, res.NotAfter.Format("2006-01-02"))
		if !*apply {
			fmt.Println("# checked only. Add -apply to install it.")
			return
		}
		requireLinuxRoot()
		fail(installPanelCert(*domain, res.Fullchain, res.Key, true))
		fail(st.SetSetting(panelssl.SettingMode, panelssl.ModeCustom))
		fail(st.SetSetting(panelssl.SettingDomain, *domain))
		fmt.Println("the panel now uses your certificate")
		printPanelURLs(*domain)
	case "ssl-off":
		if !*apply {
			fmt.Println("would go back to the self-signed certificate and stop answering to the name")
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(installer.Plan{{Name: "self-signed certificate of the panel", Actions: []installer.Action{
			installer.Run("sh", "-c", selfSignedScript),
			installer.Run("rm", "-f", panelssl.HostsFile, panelssl.ACMEConf),
			installer.Run("nginx", "-t"),
			installer.ReloadNginx(),
		}}}))
		fail(st.SetSetting(panelssl.SettingMode, panelssl.ModeSelfSigned))
		fail(st.SetSetting(panelssl.SettingDomain, ""))
		fmt.Println("the panel uses its self-signed certificate again")
	default:
		panelUsage()
	}
}

// refuseSiteName stops a name that a website already uses: two server blocks for one name on port 80 would leave one of them unreachable.
func refuseSiteName(st *store.Store, domain string) error {
	list, err := st.ListSites()
	if err != nil {
		return err
	}
	for _, s := range list {
		for _, h := range s.Hostnames {
			if h == domain {
				return fmt.Errorf("site-name: %s is the address of the website %q; the panel needs a name of its own, such as panel.%s", domain, s.Name, domain)
			}
		}
	}
	return nil
}

// renewPanelCert renews the Let's Encrypt certificate of the panel when it expires within the given days. It returns 1 when it
// failed, the way the renewal of a site is counted.
func renewPanelCert(st *store.Store, within int, apply bool, email string) int {
	mode, _ := st.Setting(panelssl.SettingMode, "")
	domain, _ := st.Setting(panelssl.SettingDomain, "")
	if mode != panelssl.ModeLetsEncrypt || panelssl.ValidDomain(domain) != nil {
		return 0
	}
	b, err := os.ReadFile(panelssl.Dir + "/fullchain.pem")
	var days int
	if err == nil {
		days, err = acme.DaysLeft(b, time.Now())
	}
	if err != nil {
		fmt.Printf("panel (%s): FAILED: %v\n", domain, err)
		return 1
	}
	switch {
	case days > within:
		fmt.Printf("panel (%s): %d days left, nothing to do\n", domain, days)
		return 0
	case !apply:
		fmt.Printf("panel (%s): %d days left, would renew (run with -apply)\n", domain, days)
		return 0
	}
	fmt.Printf("panel (%s): %d days left, renewing\n", domain, days)
	ports := listenPorts(panelNginx)
	if len(ports) == 0 {
		fmt.Printf("panel (%s): FAILED: no %s\n", domain, panelNginx)
		return 1
	}
	if err := issuePanelCert(acmeConfig(email, false), domain, ports[0], false); err != nil {
		fmt.Printf("panel (%s): FAILED: %v\n", domain, err)
		return 1
	}
	fmt.Printf("panel (%s): renewed\n", domain)
	return 0
}
