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
	fmt.Fprintln(os.Stderr, "usage: mpn mail <status|enable|disable|records|domain-add|domain-del|box-add|box-del|box-passwd|box-quota|alias-set|alias-del> [flags]   (add -h for the flags)")
	os.Exit(2)
}

// mailFirewallComment marks the firewall rules that belong to the mail server, so turning mail off removes exactly those.
const mailFirewallComment = "mpn-mail"

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
	if s.Installed {
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
