package mail

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"mpn/internal/installer"
)

// DefaultSelector is the name of the DKIM key mpn makes for every domain.
const DefaultSelector = "mpn"

// State is everything the files are made from. The panel keeps it in its database and rewrites all files from it on every change, so
// the files can be lost or edited by hand and the next "apply" puts them right.
type State struct {
	Host    string              // the name of this machine as a mail server, such as mail.example.com
	Domains []string            // domains mail is received for
	Users   []User              // mailboxes
	Aliases map[string][]string // address -> targets
	Keys    map[string]string   // domain -> private DKIM key (PEM); a domain without a key is not signed
}

// Check refuses a state the files could not express safely: a mailbox or an alias on a domain this machine does not handle would be
// accepted by Dovecot but never receive anything, and would hide a typing mistake.
func (s State) Check() error {
	if err := ValidDomain(s.Host); err != nil {
		return fmt.Errorf("mail host: %w", err)
	}
	if len(s.Domains) > MaxDomains || len(s.Users) > MaxMailboxes || len(s.Aliases) > MaxAliases {
		return fmt.Errorf("too many domains, mailboxes or aliases (at most %d, %d and %d)", MaxDomains, MaxMailboxes, MaxAliases)
	}
	have := map[string]bool{}
	for _, d := range s.Domains {
		if err := ValidDomain(d); err != nil {
			return err
		}
		if have[d] {
			return fmt.Errorf("the domain %s is listed twice", d)
		}
		have[d] = true
	}
	seen := map[string]bool{}
	for _, u := range s.Users {
		_, d, err := SplitAddress(u.Address)
		if err != nil {
			return err
		}
		if !have[d] {
			return fmt.Errorf("the mailbox %s is on a domain that is not set up here", u.Address)
		}
		if seen[u.Address] {
			return fmt.Errorf("the mailbox %s is listed twice", u.Address)
		}
		seen[u.Address] = true
	}
	for a := range s.Aliases {
		_, d, err := SplitAddress(a)
		if err != nil {
			return err
		}
		if !have[d] {
			return fmt.Errorf("the alias %s is on a domain that is not set up here", a)
		}
		if seen[a] {
			return fmt.Errorf("%s is both a mailbox and an alias", a)
		}
	}
	for d, k := range s.Keys {
		if !have[d] {
			return fmt.Errorf("there is a DKIM key for %s, which is not set up here", d)
		}
		if _, err := DKIMTextFromPrivate(k); err != nil {
			return fmt.Errorf("the DKIM key of %s: %w", d, err)
		}
	}
	return nil
}

// PackagesPlan installs the three servers. Postfix asks questions when it is installed, so the answers are given first; the file it
// makes from them is replaced by SetupPlan.
func PackagesPlan(host string) (installer.Plan, error) {
	if err := ValidDomain(host); err != nil {
		return nil, fmt.Errorf("mail host: %w", err)
	}
	return installer.Plan{{Name: "mail packages", Actions: []installer.Action{
		installer.Run("sh", "-c", "printf '%s\\n' 'postfix postfix/main_mailer_type select Internet Site' 'postfix postfix/mailname string "+host+"' | debconf-set-selections"),
		installer.Run("sh", "-c", "apt-get -o DPkg::Lock::Timeout=300 update || true"),
		installer.AptInstall("postfix", "dovecot-core", "dovecot-imapd", "dovecot-lmtpd", "opendkim", "opendkim-tools", "openssl"),
		// The servers start on their own after the packages are installed, with a configuration that is not ours. They are stopped
		// until SetupPlan has written the real one.
		installer.Run("sh", "-c", "systemctl stop postfix dovecot opendkim 2>/dev/null || true"),
	}}}, nil
}

// selfSignedScript makes the first certificate of the mail host, only when there is none, so the servers can start before a real
// certificate is issued. Mail clients will warn until it is replaced.
func selfSignedScript(host string) string {
	return "[ -s " + CertFile + " ] && [ -s " + KeyFile + " ] && exit 0\n" +
		"install -d -m 0750 -o root -g root " + CertDir + "\n" +
		"openssl req -x509 -newkey rsa:2048 -nodes -days 90 -subj '/CN=" + host + "' -addext 'subjectAltName=DNS:" + host + "' " +
		"-keyout " + KeyFile + " -out " + CertFile + " 2>/dev/null\n" +
		"chmod 0600 " + KeyFile + "\n"
}

// SetupPlan writes every file from the state and starts the servers. The configuration of each server is checked before it is
// loaded, and a failure puts the files written here back (the executor does that).
func SetupPlan(s State) (installer.Plan, error) {
	if err := s.Check(); err != nil {
		return nil, err
	}
	main, err := PostfixMain(s.Host)
	if err != nil {
		return nil, err
	}
	dove, err := Dovecot(s.Host)
	if err != nil {
		return nil, err
	}
	users, err := UsersText(s.Users)
	if err != nil {
		return nil, err
	}
	doms, err := DomainsText(s.Domains)
	if err != nil {
		return nil, err
	}
	var boxes []string
	for _, u := range s.Users {
		boxes = append(boxes, u.Address)
	}
	mb, err := MailboxesText(boxes)
	if err != nil {
		return nil, err
	}
	al, err := AliasesText(s.Aliases)
	if err != nil {
		return nil, err
	}
	var signed []string
	for d := range s.Keys {
		signed = append(signed, d)
	}
	sort.Strings(signed)
	keyTable, signTable, err := DKIMTables(signed, DefaultSelector)
	if err != nil {
		return nil, err
	}

	files := []installer.Action{
		installer.WriteFile(PostfixMainFile, main, 0o644),
		installer.WriteFile(PostfixMasterFile, PostfixMaster, 0o644),
		installer.WriteFile(DovecotFile, dove, 0o644),
		installer.WriteFile(OpenDKIMFile, OpenDKIMConf(), 0o644),
		installer.WriteFile(VDomains, doms, 0o644),
		installer.WriteFile(VMailbox, mb, 0o644),
		installer.WriteFile(VAlias, al, 0o644),
		installer.WriteFile(UsersFile, users, 0o640),
		installer.WriteFile(DKIMKeyTable, keyTable, 0o640),
		installer.WriteFile(DKIMSigningTable, signTable, 0o640),
	}
	// OpenDKIM refuses a key that a group with more than one member can read ("key data is not secure"), so the key belongs to
	// opendkim itself with mode 0600, and Postfix is never put in the opendkim group.
	perms := []installer.Action{
		installer.Run("chown", "root:dovecot", UsersFile),
		installer.Run("chown", "opendkim:opendkim", DKIMKeyTable, DKIMSigningTable),
	}
	var keys []installer.Action
	for _, d := range signed {
		keys = append(keys,
			installer.Run("install", "-d", "-m", "0750", "-o", "opendkim", "-g", "opendkim", DKIMDir+"/"+d),
			installer.WriteFile(DKIMKeyPath(d, DefaultSelector), s.Keys[d], 0o600),
			installer.Run("chown", "opendkim:opendkim", DKIMKeyPath(d, DefaultSelector)),
		)
	}

	// The key of a domain that was removed must not stay on the disk. Only the names of the domains that still have a key are put in
	// the script, and they have all been checked by ValidDomain, so nothing in it can end a quote.
	keys = append(keys, installer.Run("sh", "-c", staleKeysScript(signed)))

	setup := []installer.Action{
		installer.Run("sh", "-c", "getent passwd "+MailUser+" >/dev/null || useradd --system --uid 5000 --user-group --home-dir "+MailRoot+" --shell /usr/sbin/nologin "+MailUser),
		installer.Run("install", "-d", "-m", "0750", "-o", MailUser, "-g", MailUser, MailRoot),
		installer.Run("install", "-d", "-m", "0755", "-o", "root", "-g", "root", Dir),
		installer.Run("install", "-d", "-m", "0750", "-o", "opendkim", "-g", "opendkim", DKIMDir),
		// The socket lives in a folder Postfix can enter: opendkim runs with the group postfix (see OpenDKIMConf).
		installer.Run("install", "-d", "-m", "0750", "-o", "opendkim", "-g", "postfix", DKIMSocketDir),
		installer.Run("sh", "-c", selfSignedScript(s.Host)),
	}
	start := []installer.Action{
		installer.Run("postfix", "check"),
		installer.Run("sh", "-c", "doveconf -n >/dev/null"),
		installer.Run("opendkim", "-n", "-x", OpenDKIMFile),
		installer.Run("systemctl", "enable", "opendkim", "dovecot", "postfix"),
		installer.Run("systemctl", "restart", "opendkim", "dovecot", "postfix"),
	}
	return installer.Plan{
		{Name: "mail user and folders", Actions: setup},
		{Name: "mail configuration", Actions: slices.Concat(files, keys, perms)},
		{Name: "start the mail servers", Actions: start},
	}, nil
}

// staleKeysScript deletes the key folder of every domain that is not in keep. A folder in DKIMDir is named for a domain, so a name
// that is not in the list belongs to a domain that was removed.
func staleKeysScript(keep []string) string {
	return "cd " + DKIMDir + " 2>/dev/null || exit 0\n" +
		"for d in *; do\n" +
		"  [ -d \"$d\" ] || continue\n" +
		"  case ' " + strings.Join(keep, " ") + " ' in *\" $d \"*) ;; *) rm -rf -- \"$d\" ;; esac\n" +
		"done"
}

// ReloadPlan is the smaller change after the first setup: only the lists changed, so Postfix and Dovecot read them again and
// neither restarts. OpenDKIM is small and has no reload that rereads its tables, so it restarts.
func ReloadPlan() installer.Plan {
	return installer.Plan{{Name: "read the mail lists again", Actions: []installer.Action{
		installer.Run("systemctl", "restart", "opendkim"),
		installer.Run("systemctl", "reload", "postfix"),
		installer.Run("systemctl", "reload", "dovecot"),
	}}}
}

// UpdatePlan is what every change of a mailbox, an alias or a domain costs once mail is on: the files are written again from the
// state, checked, and the servers read them. Nothing is installed and nothing restarts apart from OpenDKIM.
func UpdatePlan(s State) (installer.Plan, error) {
	full, err := SetupPlan(s)
	if err != nil {
		return nil, err
	}
	// The first two steps of the full plan are the folders and the files; the third starts the servers, which is not needed here.
	files := installer.Plan{full[0], full[1]}
	check := installer.Step{Name: "check the mail configuration", Actions: []installer.Action{
		installer.Run("postfix", "check"),
		installer.Run("sh", "-c", "doveconf -n >/dev/null"),
		installer.Run("opendkim", "-n", "-x", OpenDKIMFile),
	}}
	return append(append(files, check), ReloadPlan()...), nil
}

// OffPlan stops the servers and keeps every mailbox, key and list, so turning mail on again loses nothing.
func OffPlan() installer.Plan {
	return installer.Plan{{Name: "turn the mail servers off", Actions: []installer.Action{
		installer.Run("sh", "-c", "systemctl disable --now postfix dovecot opendkim 2>/dev/null || true"),
	}}}
}

// MailboxDirScript removes the stored mail of an address. The path is built here from checked parts, never from a request.
func MailboxDirScript(addr string) (string, error) {
	l, d, err := SplitAddress(addr)
	if err != nil {
		return "", err
	}
	return "rm -rf -- " + MailRoot + "/" + d + "/" + l, nil
}

// Summary describes a state in one line, for logs and for the dry run.
func (s State) Summary() string {
	return fmt.Sprintf("host %s, %d domain(s), %d mailbox(es), %d alias(es)", s.Host, len(s.Domains), len(s.Users), len(s.Aliases))
}
