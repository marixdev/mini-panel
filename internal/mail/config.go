package mail

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// TLS files of the mail host. Postfix and Dovecot read the same pair.
const (
	CertDir  = "/etc/mpn/certs/_mail"
	CertFile = CertDir + "/fullchain.pem"
	KeyFile  = CertDir + "/privkey.pem"

	PostfixMainFile   = "/etc/postfix/main.cf"
	PostfixMasterFile = "/etc/postfix/master.cf"
	DovecotFile       = "/etc/dovecot/dovecot.conf"
	OpenDKIMFile      = "/etc/opendkim.conf"
	DKIMKeyTable      = Dir + "/dkim-keytable"
	DKIMSigningTable  = Dir + "/dkim-signingtable"
	DKIMSocketDir     = "/var/spool/postfix/opendkim"
	DKIMSocket        = DKIMSocketDir + "/opendkim.sock"
)

// The ports the mail server listens on from outside. The firewall opens exactly these while mail is on.
var PublicPorts = []int{25, 465, 587, 993}

// MaxMessageBytes is the largest message Postfix takes (50 MB); the web mail limits an upload to 25 MB.
const MaxMessageBytes = 52428800

// PostfixMain is main.cf. mpn owns the file while mail is on: the panel rewrites it from the lists and never asks a person to edit it.
// Recipients are looked up in text tables that Postfix reads by itself (texthash), so changing a mailbox needs no postmap and no
// database, only a reload. Mail for a mailbox goes to Dovecot over a socket (LMTP); a person signing in to send is checked by
// Dovecot too (SASL), and only on the submission ports.
func PostfixMain(host string) (string, error) {
	if err := ValidDomain(host); err != nil {
		return "", fmt.Errorf("mail host: %w", err)
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("# Written by mpn. Do not edit; the panel rewrites this file.")
	w("compatibility_level = 3.6")
	w("myhostname = %s", host)
	w("myorigin = %s", host)
	w("mydestination = localhost")
	w("inet_interfaces = all")
	w("inet_protocols = ipv4")
	w("mynetworks = 127.0.0.0/8")
	w("biff = no")
	w("append_dot_mydomain = no")
	w("smtpd_banner = $myhostname ESMTP")
	w("disable_vrfy_command = yes")
	w("message_size_limit = %d", MaxMessageBytes)
	w("mailbox_size_limit = 0")
	w("")
	w("virtual_mailbox_domains = texthash:%s", VDomains)
	w("virtual_mailbox_maps = texthash:%s", VMailbox)
	w("virtual_alias_maps = texthash:%s", VAlias)
	w("virtual_transport = lmtp:unix:private/dovecot-lmtp")
	w("")
	w("smtpd_tls_cert_file = %s", CertFile)
	w("smtpd_tls_key_file = %s", KeyFile)
	w("smtpd_tls_security_level = may")
	w("smtpd_tls_protocols = >=TLSv1.2")
	w("smtpd_tls_mandatory_protocols = >=TLSv1.2")
	w("smtp_tls_security_level = may")
	w("smtp_tls_protocols = >=TLSv1.2")
	w("smtpd_sasl_type = dovecot")
	w("smtpd_sasl_path = private/auth")
	w("smtpd_sasl_auth_enable = no")
	w("")
	w("smtpd_helo_required = yes")
	w("smtpd_relay_restrictions = permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination")
	w("smtpd_recipient_restrictions = reject_non_fqdn_recipient, reject_unknown_recipient_domain, permit_mynetworks, permit_sasl_authenticated, reject_unauth_destination")
	w("")
	w("smtpd_milters = unix:opendkim/opendkim.sock")
	w("non_smtpd_milters = $smtpd_milters")
	w("milter_default_action = accept")
	w("milter_protocol = 6")
	return b.String(), nil
}

// PostfixMaster is master.cf: the usual services, and two for people who sign in to send (587 with STARTTLS, 465 with TLS from the
// first byte). Both demand a login and TLS, so the server is never an open relay.
const PostfixMaster = `# Written by mpn. Do not edit; the panel rewrites this file.
smtp      inet  n       -       y       -       -       smtpd
submission inet n       -       y       -       -       smtpd
  -o syslog_name=postfix/submission
  -o smtpd_tls_security_level=encrypt
  -o smtpd_sasl_auth_enable=yes
  -o smtpd_tls_auth_only=yes
  -o smtpd_client_restrictions=permit_sasl_authenticated,reject
  -o smtpd_relay_restrictions=permit_sasl_authenticated,reject
smtps     inet  n       -       y       -       -       smtpd
  -o syslog_name=postfix/smtps
  -o smtpd_tls_wrappermode=yes
  -o smtpd_sasl_auth_enable=yes
  -o smtpd_client_restrictions=permit_sasl_authenticated,reject
  -o smtpd_relay_restrictions=permit_sasl_authenticated,reject
pickup    unix  n       -       y       60      1       pickup
cleanup   unix  n       -       y       -       0       cleanup
qmgr      unix  n       -       n       300     1       qmgr
tlsmgr    unix  -       -       y       1000?   1       tlsmgr
rewrite   unix  -       -       y       -       -       trivial-rewrite
bounce    unix  -       -       y       -       0       bounce
defer     unix  -       -       y       -       0       bounce
trace     unix  -       -       y       -       0       bounce
verify    unix  -       -       y       -       1       verify
flush     unix  n       -       y       1000?   0       flush
proxymap  unix  -       -       n       -       -       proxymap
proxywrite unix -       -       n       -       1       proxymap
smtp      unix  -       -       y       -       -       smtp
relay     unix  -       -       y       -       -       smtp
showq     unix  n       -       y       -       -       showq
error     unix  -       -       y       -       -       error
retry     unix  -       -       y       -       -       error
discard   unix  -       -       y       -       -       discard
local     unix  -       n       n       -       -       local
virtual   unix  -       n       n       -       -       virtual
lmtp      unix  -       -       y       -       -       lmtp
anvil     unix  -       -       y       -       1       anvil
scache    unix  -       -       y       -       1       scache
postlog   unix-dgram n  -       n       -       1       postlogd
`

// Dovecot is dovecot.conf. Mailboxes are Maildir folders under MailRoot, one per address; the password list is a file, so no database
// is needed. Port 143 listens on the loopback only, for the web mail on this machine; from outside only IMAPS (993) is open.
// v24 asks for the syntax of Dovecot 2.4 (Debian 13, Ubuntu 26.04), which renamed most settings and refuses the 2.3 file.
func Dovecot(host string, v24 bool) (string, error) {
	if err := ValidDomain(host); err != nil {
		return "", fmt.Errorf("mail host: %w", err)
	}
	if v24 {
		return dovecot24(), nil
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("# Written by mpn. Do not edit; the panel rewrites this file.")
	w("protocols = imap lmtp")
	w("listen = *")
	w("mail_location = maildir:%s/%%d/%%n", MailRoot)
	w("mail_uid = %s", MailUser)
	w("mail_gid = %s", MailUser)
	w("first_valid_uid = 100")
	w("mail_plugins = quota")
	w("disable_plaintext_auth = yes")
	w("auth_mechanisms = plain login")
	w("auth_username_format = %%Lu")
	w("ssl = required")
	w("ssl_cert = <%s", CertFile)
	w("ssl_key = <%s", KeyFile)
	w("ssl_min_protocol = TLSv1.2")
	w("")
	w("passdb {")
	w("  driver = passwd-file")
	w("  args = username_format=%%Lu %s", UsersFile)
	w("}")
	w("userdb {")
	w("  driver = passwd-file")
	w("  args = username_format=%%Lu %s", UsersFile)
	w("  default_fields = uid=%s gid=%s home=%s/%%d/%%n", MailUser, MailUser, MailRoot)
	w("}")
	w("")
	w("namespace inbox {")
	w("  inbox = yes")
	for _, m := range [][2]string{{"Drafts", `\Drafts`}, {"Sent", `\Sent`}, {"Trash", `\Trash`}, {"Junk", `\Junk`}} {
		w("  mailbox %s {", m[0])
		w("    special_use = %s", m[1])
		w("    auto = subscribe")
		w("  }")
	}
	w("}")
	w("")
	w("service imap-login {")
	w("  inet_listener imap {")
	w("    address = 127.0.0.1")
	w("    port = 143")
	w("  }")
	w("  inet_listener imaps {")
	w("    port = 993")
	w("    ssl = yes")
	w("  }")
	w("}")
	w("service lmtp {")
	w("  unix_listener /var/spool/postfix/private/dovecot-lmtp {")
	w("    mode = 0600")
	w("    user = postfix")
	w("    group = postfix")
	w("  }")
	w("}")
	w("service auth {")
	w("  unix_listener /var/spool/postfix/private/auth {")
	w("    mode = 0660")
	w("    user = postfix")
	w("    group = postfix")
	w("  }")
	w("}")
	w("protocol imap {")
	w("  mail_plugins = $mail_plugins imap_quota")
	w("}")
	w("plugin {")
	w("  quota = maildir:User quota")
	w("}")
	return b.String(), nil
}

// dovecot24 is the same configuration as Dovecot writes for 2.3, in the syntax of 2.4: the first setting must name the version, the
// mail location is split in driver and path, variables are %{user | domain}, and quota is a named block. Checked with doveconf 2.4.1.
func dovecot24() string {
	home := MailRoot + "/%{user | domain}/%{user | username}"
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	w("dovecot_config_version = 2.4.0")
	w("dovecot_storage_version = 2.4.0")
	w("# Written by mpn. Do not edit; the panel rewrites this file.")
	w("protocols = imap lmtp")
	w("listen = *")
	w("mail_driver = maildir")
	w("mail_home = %s", home)
	w("mail_path = ~")
	w("mail_uid = %s", MailUser)
	w("mail_gid = %s", MailUser)
	w("first_valid_uid = 100")
	w("auth_allow_cleartext = no")
	w("auth_mechanisms = plain login")
	w("auth_username_format = %%{user | lower}")
	w("ssl = required")
	w("ssl_server_cert_file = %s", CertFile)
	w("ssl_server_key_file = %s", KeyFile)
	w("ssl_min_protocol = TLSv1.2")
	w("mail_plugins {")
	w("  quota = yes")
	w("}")
	w("")
	w("passdb passwd-file {")
	w("  passwd_file_path = %s", UsersFile)
	w("}")
	w("userdb passwd-file {")
	w("  passwd_file_path = %s", UsersFile)
	w("  fields {")
	w("    uid:default = %s", MailUser)
	w("    gid:default = %s", MailUser)
	w("    home:default = %s", home)
	w("  }")
	w("}")
	w("")
	w("namespace inbox {")
	w("  inbox = yes")
	for _, m := range [][2]string{{"Drafts", `\Drafts`}, {"Sent", `\Sent`}, {"Trash", `\Trash`}, {"Junk", `\Junk`}} {
		w("  mailbox %s {", m[0])
		w("    special_use = %s", m[1])
		w("    auto = subscribe")
		w("  }")
	}
	w("}")
	w("")
	w("service imap-login {")
	w("  inet_listener imap {")
	w("    listen = 127.0.0.1")
	w("    port = 143")
	w("  }")
	w("  inet_listener imaps {")
	w("    port = 993")
	w("    ssl = yes")
	w("  }")
	w("}")
	w("service lmtp {")
	w("  unix_listener /var/spool/postfix/private/dovecot-lmtp {")
	w("    mode = 0600")
	w("    user = postfix")
	w("    group = postfix")
	w("  }")
	w("}")
	w("service auth {")
	w("  unix_listener /var/spool/postfix/private/auth {")
	w("    mode = 0660")
	w("    user = postfix")
	w("    group = postfix")
	w("  }")
	w("}")
	w("protocol imap {")
	w("  mail_plugins {")
	w("    imap_quota = yes")
	w("  }")
	w("}")
	w(`quota "User quota" {`)
	w("  driver = maildir")
	w("}")
	return b.String()
}

// User is one mailbox as the Dovecot password list holds it.
type User struct {
	Address string
	Hash    string
	QuotaMB int // 0 means no limit
}

// UsersText is the password list. Each line is address:hash:uid:gid:gecos:home:shell:extra, and only the first two fields and the
// extra one are filled. A hash that is not exactly what HashPassword makes is refused, so no stored value can add a field or a line.
func UsersText(users []User, v24 bool) (string, error) {
	sorted := append([]User(nil), users...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Address < sorted[j].Address })
	var b strings.Builder
	b.WriteString("# Written by mpn. Do not edit.\n")
	for _, u := range sorted {
		if _, _, err := SplitAddress(u.Address); err != nil {
			return "", err
		}
		if !ValidHash(u.Hash) {
			return "", fmt.Errorf("the stored password of %s is not a hash made by mpn", u.Address)
		}
		if err := ValidQuota(u.QuotaMB); err != nil {
			return "", err
		}
		extra := ""
		if u.QuotaMB > 0 {
			extra = "userdb_quota_rule=*:storage=" + strconv.Itoa(u.QuotaMB) + "M"
			if v24 { // 2.4 replaced quota rules with a size setting of the named quota
				extra = "userdb_quota_storage_size=" + strconv.Itoa(u.QuotaMB) + "M"
			}
		}
		// user:password:uid:gid:gecos:home:shell:extra is eight fields, seven colons; uid, gid and home come from the userdb defaults.
		fmt.Fprintf(&b, "%s:%s::::::%s\n", u.Address, u.Hash, extra)
	}
	return b.String(), nil
}

// DomainsText lists the domains Postfix accepts mail for, one per line, as a table whose value is not used.
func DomainsText(domains []string) (string, error) {
	sorted := append([]string(nil), domains...)
	sort.Strings(sorted)
	var b strings.Builder
	b.WriteString("# Written by mpn. Do not edit.\n")
	for _, d := range sorted {
		if err := ValidDomain(d); err != nil {
			return "", err
		}
		b.WriteString(d + " OK\n")
	}
	return b.String(), nil
}

// MailboxesText maps each address to its Maildir. Postfix uses the table to refuse mail for an address that does not exist, at the
// door, instead of accepting it and bouncing it later.
func MailboxesText(addrs []string) (string, error) {
	sorted := append([]string(nil), addrs...)
	sort.Strings(sorted)
	var b strings.Builder
	b.WriteString("# Written by mpn. Do not edit.\n")
	for _, a := range sorted {
		l, d, err := SplitAddress(a)
		if err != nil {
			return "", err
		}
		b.WriteString(a + " " + d + "/" + l + "/\n")
	}
	return b.String(), nil
}

// AliasesText maps an address to the addresses its mail is forwarded to. A target may be on any domain.
func AliasesText(aliases map[string][]string) (string, error) {
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Written by mpn. Do not edit.\n")
	for _, k := range keys {
		if _, _, err := SplitAddress(k); err != nil {
			return "", err
		}
		targets := aliases[k]
		if len(targets) == 0 {
			return "", fmt.Errorf("the alias %s has no target", k)
		}
		for _, t := range targets {
			if _, _, err := SplitAddress(t); err != nil {
				return "", fmt.Errorf("alias %s: %w", k, err)
			}
		}
		b.WriteString(k + " " + strings.Join(targets, ", ") + "\n")
	}
	return b.String(), nil
}

// OpenDKIMConf is opendkim.conf: it signs mail that leaves, with the key of the sender's domain, over a socket Postfix can reach.
func OpenDKIMConf() string {
	return "# Written by mpn. Do not edit; the panel rewrites this file.\n" +
		"Syslog yes\nUMask 007\nMode s\nCanonicalization relaxed/simple\nOversignHeaders From\n" +
		"KeyTable " + DKIMKeyTable + "\nSigningTable refile:" + DKIMSigningTable + "\n" +
		"Socket local:" + DKIMSocket + "\nPidFile /run/opendkim/opendkim.pid\n" +
		// The group postfix makes the socket reachable by Postfix without putting Postfix in the group that owns the keys.
		"UserID opendkim:postfix\n"
}

// DKIMKeyPath is where the private key of a domain lives.
func DKIMKeyPath(domain, selector string) string {
	return DKIMDir + "/" + domain + "/" + selector + ".private"
}

// DKIMTables are the key table and the signing table for the domains that have a key.
func DKIMTables(domains []string, selector string) (keyTable, signingTable string, err error) {
	if err := ValidSelector(selector); err != nil {
		return "", "", err
	}
	sorted := append([]string(nil), domains...)
	sort.Strings(sorted)
	var k, s strings.Builder
	k.WriteString("# Written by mpn. Do not edit.\n")
	s.WriteString("# Written by mpn. Do not edit.\n")
	for _, d := range sorted {
		if err := ValidDomain(d); err != nil {
			return "", "", err
		}
		name := selector + "._domainkey." + d
		fmt.Fprintf(&k, "%s %s:%s:%s\n", name, d, selector, DKIMKeyPath(d, selector))
		fmt.Fprintf(&s, "*@%s %s\n", d, name)
	}
	return k.String(), s.String(), nil
}
