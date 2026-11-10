package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"mpn/internal/firewall"
	"mpn/internal/installer"
	"mpn/internal/mail"
	"mpn/internal/store"
)

func mailUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn mail <status|enable|disable|ssl|webmail|records|domain-add|domain-del|box-add|box-del|box-passwd|box-quota|alias-set|alias-del> [flags]   (add -h for the flags)")
	os.Exit(2)
}

// mailFirewallComment marks the firewall rules that belong to the mail server, so turning mail off removes exactly those. The web mail
// has a comment of its own, because it can be turned off while the mail server stays on.
const (
	mailFirewallComment = "mpn-mail"
	webmailFirewallTag  = "mpn-mail-web"
	settingWebmail      = "mail_webmail" // "on" or "off"
)

// mailWebInfo is the part of the status that is about the web mail.
type mailWebInfo struct {
	On        bool   `json:"on"`        // a person turned it on
	Installed bool   `json:"installed"` // the program is on the machine
	Active    bool   `json:"active"`    // the socket that starts it is open
	Port      int    `json:"port"`      // where it is reached; 0 when the panel's port is unknown
	URL       string `json:"url"`       // its address without a port, on the name of the mail host, when nginx serves it there
}

func panelPortOf() int {
	if p := listenPorts(panelNginx); len(p) > 0 {
		return p[0]
	}
	return 0
}

func currentWebmail(st *store.Store) mailWebInfo {
	v, _ := st.Setting(settingWebmail, "off")
	w := mailWebInfo{On: v == "on", Installed: fileExists(mail.WebmailBinary)}
	if pp := panelPortOf(); pp > 0 {
		w.Port = mail.WebmailPort(pp)
	}
	if host, _ := st.MailHost(); host != "" && fileExists(mail.WebmailDomainFile) {
		w.URL = "https://" + host
	}
	w.Active = out(3*time.Second, "systemctl", "is-active", "mpn-mail.socket") == "active"
	return w
}

// webmailFirewall opens or closes the port of the web mail, the way mailFirewallOpen does for the mail ports. The panel's own ports
// are always open; this one is not, so with the firewall on it needs a rule.
func webmailFirewall(st *store.Store, open bool, port int) error {
	rules, err := st.FirewallRules()
	if err != nil {
		return err
	}
	for _, r := range rules {
		if r.Comment == webmailFirewallTag {
			if err := st.DeleteFirewallRule(r.ID); err != nil {
				return err
			}
		}
	}
	if open && port > 0 {
		r := firewall.Rule{Action: firewall.Allow, Proto: "tcp", Port: fmt.Sprint(port), Comment: webmailFirewallTag}
		if _, err := st.AddFirewallRule(r); err != nil && !errors.Is(err, store.ErrExists) {
			return err
		}
	}
	return mailFirewallReload(st)
}

type mailDomainInfo struct {
	Name      string        `json:"name"`
	HasKey    bool          `json:"has_key"`
	Mailboxes int           `json:"mailboxes"`
	Aliases   int           `json:"aliases"`
	Records   []mail.Record `json:"records"`
}

type mailBoxInfo struct {
	Address string `json:"address"`
	QuotaMB int    `json:"quota_mb"`
}

type mailAliasInfo struct {
	Address string   `json:"address"`
	Targets []string `json:"targets"`
}

// mailStatus is the answer of "mpn mail status -json". Passwords, hashes and private keys are never part of it.
type mailStatus struct {
	On        bool             `json:"on"`
	Host      string           `json:"host"`
	Installed bool             `json:"installed"`
	Services  map[string]bool  `json:"services"` // name -> running
	Ports     []int            `json:"ports"`
	Cert      mailCertInfo     `json:"cert"`
	Webmail   mailWebInfo      `json:"webmail"`
	Domains   []mailDomainInfo `json:"domains"`
	Mailboxes []mailBoxInfo    `json:"mailboxes"`
	Aliases   []mailAliasInfo  `json:"aliases"`
}

func mailInstalled() bool {
	return fileExists("/usr/sbin/postfix") && fileExists("/usr/sbin/dovecot") && fileExists("/usr/sbin/opendkim")
}

func currentMail(st *store.Store) mailStatus {
	on, _ := st.MailOn()
	host, _ := st.MailHost()
	s := mailStatus{On: on, Host: host, Installed: mailInstalled(), Services: map[string]bool{}, Ports: mail.PublicPorts,
		Domains: []mailDomainInfo{}, Mailboxes: []mailBoxInfo{}, Aliases: []mailAliasInfo{}}
	s.Webmail = currentWebmail(st)
	if s.Installed {
		s.Cert = currentMailCert(st, host)
		for _, svc := range []string{"postfix", "dovecot", "opendkim"} {
			s.Services[svc] = out(3*time.Second, "systemctl", "is-active", svc) == "active"
		}
	}
	doms, _ := st.MailDomains()
	for _, d := range doms {
		info := mailDomainInfo{Name: d.Name, HasKey: d.HasKey, Mailboxes: d.Mailboxes, Aliases: d.Aliases, Records: []mail.Record{}}
		if d.HasKey && host != "" {
			if key, err := st.MailDKIMKey(d.Name); err == nil {
				if txt, err := mail.DKIMTextFromPrivate(key); err == nil {
					info.Records = mail.Records(d.Name, host, mail.DefaultSelector, txt)
				}
			}
		}
		s.Domains = append(s.Domains, info)
	}
	boxes, _ := st.Mailboxes("")
	for _, b := range boxes {
		s.Mailboxes = append(s.Mailboxes, mailBoxInfo{Address: b.Address, QuotaMB: b.QuotaMB})
	}
	aliases, _ := st.MailAliases("")
	for _, a := range aliases {
		s.Aliases = append(s.Aliases, mailAliasInfo{Address: a.Address, Targets: a.Targets})
	}
	return s
}

// readMailPassword takes the password of a mailbox from standard input, one line, and checks it. It never comes from an argument.
func readMailPassword() string {
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	pw := strings.TrimRight(line, "\r\n")
	if pw == "" {
		fail(errors.New("no password on standard input"))
	}
	fail(mail.ValidPassword(pw))
	return pw
}

// mailFirewallOpen adds the rules for the mail ports (a rule that exists is left alone) and, when the firewall is on, loads them.
func mailFirewallOpen(st *store.Store) error {
	for _, p := range mail.PublicPorts {
		r := firewall.Rule{Action: firewall.Allow, Proto: "tcp", Port: fmt.Sprint(p), Comment: mailFirewallComment}
		if _, err := st.AddFirewallRule(r); err != nil && !errors.Is(err, store.ErrExists) {
			return err
		}
	}
	return mailFirewallReload(st)
}

// mailFirewallClose removes only the rules the mail server added.
func mailFirewallClose(st *store.Store) error {
	rules, err := st.FirewallRules()
	if err != nil {
		return err
	}
	for _, r := range rules {
		if r.Comment == mailFirewallComment {
			if err := st.DeleteFirewallRule(r.ID); err != nil {
				return err
			}
		}
	}
	return mailFirewallReload(st)
}

func mailFirewallReload(st *store.Store) error {
	if on, _ := st.FirewallOn(); !on {
		return nil
	}
	rules, err := st.FirewallRules()
	if err != nil {
		return err
	}
	return runPlanQuiet(loadPlan(rules))
}

// mailApplyState writes the files from the stored state and makes the servers read them. It does nothing while mail is off, because
// then nothing is installed and the state is only kept for when it is turned on.
func mailApplyState(st *store.Store) error {
	on, _ := st.MailOn()
	if !on {
		return nil
	}
	state, err := st.MailState()
	if err != nil {
		return err
	}
	plan, err := mail.UpdatePlan(state)
	if err != nil {
		return err
	}
	return runPlanQuiet(plan)
}

func printPlans(plans ...installer.Plan) {
	for _, p := range plans {
		fmt.Print(p)
	}
}

// runMail manages the mail server: Postfix, Dovecot and OpenDKIM. Nothing is installed until a person turns mail on; turning it off
// stops the servers and keeps every mailbox, key and list.
func runMail(args []string) {
	if len(args) == 0 {
		mailUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("mail "+sub, flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	asJSON := fs.Bool("json", false, "status: print JSON")
	host := fs.String("host", "", "enable: the name of this machine as a mail server, such as mail.example.com")
	domain := fs.String("domain", "", "the mail domain")
	address := fs.String("address", "", "the mailbox or alias address")
	targets := fs.String("targets", "", "alias-set: the addresses its mail goes to, separated by commas")
	webOn := fs.Bool("on", true, "webmail: turn the web mail on (false turns it off)")
	binary := fs.String("binary", "", "webmail: install the program from this file on the machine instead of downloading it (the checksum is still checked)")
	email := fs.String("email", "", "ssl: contact address for Let's Encrypt (optional)")
	staging := fs.Bool("staging", false, "ssl: use the Let's Encrypt test CA (for trials; mail clients do not trust the result)")
	quota := fs.Int("quota", 0, "box-add, box-quota: size limit in MB, 0 for none")
	purge := fs.Bool("purge", false, "domain-del, box-del: also delete the stored mail")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])
	st := openStore(*db)
	defer st.Close()

	switch sub {
	case "status":
		s := currentMail(st)
		if *asJSON {
			printJSON(s)
			return
		}
		fmt.Printf("mail: on %v, host %q, installed %v, services %v\n", s.On, s.Host, s.Installed, s.Services)
		for _, d := range s.Domains {
			fmt.Printf("domain %s\tmailboxes %d\taliases %d\tdkim %v\n", d.Name, d.Mailboxes, d.Aliases, d.HasKey)
		}
		return

	case "records":
		fail(mail.ValidDomain(*domain))
		s := currentMail(st)
		for _, d := range s.Domains {
			if d.Name == *domain {
				if *asJSON {
					printJSON(d.Records)
					return
				}
				for _, r := range d.Records {
					fmt.Printf("%s\t%s\t%s\n", r.Type, r.Name, r.Value)
				}
				return
			}
		}
		fail(fmt.Errorf("the domain %s is not set up for mail", *domain))

	case "enable":
		h := *host
		if h == "" {
			h, _ = st.MailHost()
		}
		fail(mail.ValidDomain(h))
		if h == "localhost" {
			fail(errors.New("the mail host needs a real name"))
		}
		if !*apply {
			pk, err := mail.PackagesPlan(h)
			fail(err)
			state, _ := st.MailState()
			state.Host = h
			setup, err := mail.SetupPlan(state)
			fail(err)
			printPlans(pk, setup)
			fmt.Printf("# checked only. Mail would open ports %v. Add -apply to install and start it.\n", mail.PublicPorts)
			return
		}
		requireLinuxRoot()
		if c := conflictingMTA(); c != "" {
			fail(fmt.Errorf("mail-conflict: %s is already running on port 25; remove it first", c))
		}
		fail(st.SetMailHost(h))
		pk, err := mail.PackagesPlan(h)
		fail(err)
		fail(runPlanQuiet(pk))
		state, err := st.MailState()
		fail(err)
		setup, err := mail.SetupPlan(state)
		fail(err)
		fail(runPlanQuiet(setup))
		fail(st.SetMailOn(true))
		fail(mailFirewallOpen(st))
		fmt.Printf("mail is on for %s\n", h)

	case "ssl":
		mailSSL(st, *email, *staging, *apply)

	case "webmail":
		if on, _ := st.MailOn(); !on {
			fail(errors.New("mail-off: turn mail on first; the web mail reads the mailboxes of this server"))
		}
		pp := panelPortOf()
		if pp == 0 {
			fail(errors.New("webmail-nopanel: the web mail is served on a port next to the panel's, and the panel is not installed on this machine"))
		}
		if !*webOn {
			if !*apply {
				printPlans(mail.WebmailOffPlan())
				fmt.Println("# checked only. Add -apply to turn the web mail off.")
				return
			}
			requireLinuxRoot()
			fail(runPlanQuiet(mail.WebmailOffPlan()))
			fail(st.SetSetting(settingWebmail, "off"))
			fail(webmailFirewall(st, false, 0))
			fail(syncFront(st))
			fmt.Println("the web mail is off; its settings are kept")
			return
		}
		plan, err := mail.WebmailPlan(pp, *binary)
		fail(err)
		if !*apply {
			printPlans(plan)
			fmt.Printf("# checked only. The web mail would be at port %d. Add -apply to install it.\n", mail.WebmailPort(pp))
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(plan))
		fail(st.SetSetting(settingWebmail, "on"))
		fail(syncFront(st))
		fail(webmailFirewall(st, true, mail.WebmailPort(pp)))
		fmt.Printf("the web mail is on, at port %d of this server\n", mail.WebmailPort(pp))

	case "disable":
		if !*apply {
			printPlans(mail.OffPlan())
			fmt.Println("# checked only. Add -apply to stop the mail servers. Mailboxes and keys are kept.")
			return
		}
		requireLinuxRoot()
		fail(runPlanQuiet(mail.OffPlan()))
		fail(st.SetMailOn(false))
		fail(mailFirewallClose(st))
		// The web mail has nothing to read once the servers stop, so it goes too. Its settings stay.
		if v, _ := st.Setting(settingWebmail, "off"); v == "on" {
			fail(runPlanQuiet(mail.WebmailOffPlan()))
			fail(st.SetSetting(settingWebmail, "off"))
			fail(webmailFirewall(st, false, 0))
		}
		fail(syncFront(st))
		fmt.Println("mail is off; mailboxes, keys and lists are kept")

	case "domain-add":
		fail(mail.ValidDomain(*domain))
		priv, _, err := mail.NewDKIMKey()
		fail(err)
		if !*apply {
			fmt.Printf("# checked only. Would set up mail for %s with a new DKIM key.\n", *domain)
			return
		}
		requireLinuxRoot()
		fail(st.AddMailDomain(*domain, priv))
		if err := mailApplyState(st); err != nil {
			_ = st.DeleteMailDomain(*domain)
			fail(fmt.Errorf("the change was undone: %w", err))
		}
		fmt.Printf("mail domain %s added\n", *domain)

	case "domain-del":
		fail(mail.ValidDomain(*domain))
		boxes, _ := st.Mailboxes(*domain)
		if !*apply {
			fmt.Printf("# checked only. Would remove %s with %d mailbox(es) and its aliases.\n", *domain, len(boxes))
			return
		}
		requireLinuxRoot()
		fail(st.DeleteMailDomain(*domain))
		fail(mailApplyState(st))
		if *purge {
			fail(runPlanQuiet(installer.Plan{{Name: "delete stored mail", Actions: []installer.Action{installer.Run("rm", "-rf", "--", mail.MailRoot+"/"+*domain)}}}))
		}
		fmt.Printf("mail domain %s removed\n", *domain)

	case "box-add":
		_, _, err := mail.SplitAddress(*address)
		fail(err)
		fail(mail.ValidQuota(*quota))
		if !*apply {
			fmt.Printf("# checked only. Would create the mailbox %s (limit %d MB). The password is read from standard input.\n", *address, *quota)
			return
		}
		requireLinuxRoot()
		h, err := mail.HashPassword(readMailPassword())
		fail(err)
		fail(st.AddMailbox(*address, h, *quota))
		if err := mailApplyState(st); err != nil {
			_ = st.DeleteMailbox(*address)
			fail(fmt.Errorf("the change was undone: %w", err))
		}
		fmt.Printf("mailbox %s created\n", *address)

	case "box-del":
		_, _, err := mail.SplitAddress(*address)
		fail(err)
		if !*apply {
			fmt.Printf("# checked only. Would remove the mailbox %s.\n", *address)
			return
		}
		requireLinuxRoot()
		fail(st.DeleteMailbox(*address))
		fail(mailApplyState(st))
		if *purge {
			script, err := mail.MailboxDirScript(*address)
			fail(err)
			fail(runPlanQuiet(installer.Plan{{Name: "delete stored mail", Actions: []installer.Action{installer.Run("sh", "-c", script)}}}))
		}
		fmt.Printf("mailbox %s removed\n", *address)

	case "box-passwd":
		_, _, err := mail.SplitAddress(*address)
		fail(err)
		if !*apply {
			fmt.Printf("# checked only. Would set a new password for %s, read from standard input.\n", *address)
			return
		}
		requireLinuxRoot()
		h, err := mail.HashPassword(readMailPassword())
		fail(err)
		fail(st.SetMailboxPassword(*address, h))
		fail(mailApplyState(st))
		fmt.Printf("the password of %s was changed\n", *address)

	case "box-quota":
		_, _, err := mail.SplitAddress(*address)
		fail(err)
		fail(mail.ValidQuota(*quota))
		if !*apply {
			fmt.Printf("# checked only. Would limit %s to %d MB.\n", *address, *quota)
			return
		}
		requireLinuxRoot()
		fail(st.SetMailboxQuota(*address, *quota))
		fail(mailApplyState(st))
		fmt.Printf("the limit of %s is %d MB\n", *address, *quota)

	case "alias-set":
		_, _, err := mail.SplitAddress(*address)
		fail(err)
		list := strings.Split(*targets, ",")
		if !*apply {
			fmt.Printf("# checked only. Would send mail for %s to %v.\n", *address, list)
			return
		}
		requireLinuxRoot()
		fail(st.SetMailAlias(*address, list))
		fail(mailApplyState(st))
		fmt.Printf("alias %s saved\n", *address)

	case "alias-del":
		_, _, err := mail.SplitAddress(*address)
		fail(err)
		if !*apply {
			fmt.Printf("# checked only. Would remove the alias %s.\n", *address)
			return
		}
		requireLinuxRoot()
		fail(st.DeleteMailAlias(*address))
		fail(mailApplyState(st))
		fmt.Printf("alias %s removed\n", *address)

	default:
		mailUsage()
	}
}

// conflictingMTA names a mail server other than the ones mpn manages that is listening on port 25. Installing Postfix next to Exim
// (the default on some images) would leave two servers fighting for the port.
func conflictingMTA() string {
	for _, name := range []string{"exim4", "sendmail", "opensmtpd"} {
		if out(3*time.Second, "systemctl", "is-active", name) == "active" {
			return name
		}
	}
	return ""
}
