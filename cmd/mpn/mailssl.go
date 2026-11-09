package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"mpn/internal/acme"
	"mpn/internal/certs"
	"mpn/internal/dnscheck"
	"mpn/internal/installer"
	"mpn/internal/mail"
	"mpn/internal/store"
)

// mailCertInfo describes the certificate the mail servers use now.
type mailCertInfo struct {
	Mode   string `json:"mode"`
	Days   int    `json:"days"`
	Issuer string `json:"issuer"`
	Covers bool   `json:"covers"`
}

func currentMailCert(st *store.Store, host string) mailCertInfo {
	mode, _ := st.Setting(mail.SettingSSL, mail.SSLSelfSigned)
	info := mailCertInfo{Mode: mode}
	if b, err := os.ReadFile(mail.CertFile); err == nil {
		if d, err := acme.DaysLeft(b, time.Now()); err == nil {
			info.Days = d
		}
		if c, err := certs.Describe(b); err == nil {
			info.Issuer = c.Issuer
		}
		info.Covers = host != "" && certs.Covers(b, []string{host})
	}
	return info
}

// installMailCert writes the certificate where Postfix and Dovecot read it and makes them read it again. postfix check runs first,
// and a failure puts the old files back (the executor does that).
func installMailCert(chain, key []byte) error {
	return runPlanQuiet(installer.Plan{{Name: "certificate of the mail host", Actions: []installer.Action{
		installer.Run("install", "-d", "-m", "0750", "-o", "root", "-g", "root", mail.CertDir),
		installer.WriteFile(mail.CertFile, string(chain), 0o644),
		installer.WriteFile(mail.KeyFile, string(key), 0o600),
		installer.Run("postfix", "check"),
		installer.Run("systemctl", "reload", "postfix", "dovecot"),
	}}})
}

// issueMailCert gets a Let's Encrypt certificate for the mail host. The block that answers the challenge is written first, because
// the check that the name leads here asks that block for a file. When it was not there before and the attempt fails, it is removed.
func issueMailCert(cfg acme.Config, host string, checkDNS bool) error {
	if !fileExists("/etc/nginx/conf.d") {
		return errors.New("mail-nonginx: nginx is not installed, and Let's Encrypt needs it on port 80 to check the name")
	}
	existed := fileExists(mail.ACMEConf)
	conf, err := mail.ACMEConfig(host)
	if err != nil {
		return err
	}
	undo := func() {
		if !existed {
			_ = runPlanQuiet(installer.Plan{{Name: "undo", Actions: []installer.Action{installer.Run("rm", "-f", mail.ACMEConf), installer.ReloadNginx()}}})
		}
	}
	if err := runPlanQuiet(installer.Plan{{Name: "challenge for the name of the mail host", Actions: []installer.Action{
		installer.Run("install", "-d", "-m", "0755", "/var/lib/mpn/acme"),
		installer.WriteFile(mail.ACMEConf, conf, 0o644),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}}}); err != nil {
		return err
	}
	if checkDNS {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		f := dnscheck.Check(ctx, realDNS(), []string{host})
		cancel()
		if f != nil {
			undo()
			return f
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	chain, key, err := acme.Issue(ctx, cfg, []string{host})
	if err != nil {
		undo()
		return err
	}
	return installMailCert(chain, key)
}

// mailSSL gets the certificate and records it. The host is the one chosen when mail was turned on.
func mailSSL(st *store.Store, email string, staging, apply bool) {
	on, _ := st.MailOn()
	host, _ := st.MailHost()
	if !on || host == "" {
		fail(errors.New("mail-off: turn mail on first; the certificate is for the name of the mail host"))
	}
	if !apply {
		fmt.Printf("would check that %s leads to this server, get a certificate from Let's Encrypt, and use it for Postfix and Dovecot\n", host)
		return
	}
	requireLinuxRoot()
	fail(issueMailCert(acmeConfig(email, staging), host, true))
	fail(st.SetSetting(mail.SettingSSL, mail.SSLLetsEncrypt))
	fmt.Printf("the mail host %s now has a Let's Encrypt certificate; mpn renews it\n", host)
}

// renewMailCert renews the Let's Encrypt certificate of the mail host when it expires within the given days. It returns 1 when it
// failed, the way the renewal of a site is counted.
func renewMailCert(st *store.Store, within int, apply bool, email string) int {
	mode, _ := st.Setting(mail.SettingSSL, "")
	host, _ := st.MailHost()
	if on, _ := st.MailOn(); !on || mode != mail.SSLLetsEncrypt || mail.ValidDomain(host) != nil {
		return 0
	}
	b, err := os.ReadFile(mail.CertFile)
	var days int
	if err == nil {
		days, err = acme.DaysLeft(b, time.Now())
	}
	if err != nil {
		fmt.Printf("mail (%s): FAILED: %v\n", host, err)
		return 1
	}
	switch {
	case days > within:
		fmt.Printf("mail (%s): %d days left, nothing to do\n", host, days)
		return 0
	case !apply:
		fmt.Printf("mail (%s): %d days left, would renew (run with -apply)\n", host, days)
		return 0
	}
	fmt.Printf("mail (%s): %d days left, renewing\n", host, days)
	if err := issueMailCert(acmeConfig(email, false), host, false); err != nil {
		fmt.Printf("mail (%s): FAILED: %v\n", host, err)
		return 1
	}
	fmt.Printf("mail (%s): renewed\n", host)
	return 0
}
