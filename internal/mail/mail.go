// Package mail describes the mail server of a machine: Postfix receives and sends, Dovecot keeps the mailboxes and answers IMAP,
// OpenDKIM signs what leaves. Like the firewall and security packages it only checks values and makes text and plans; the work
// itself is done by the caller, as root, through the helper.
package mail

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"

	"mpn/internal/distro"
)

// hashBcrypt is the {BLF-CRYPT} form Dovecot reads: bcrypt with the $2y$ prefix, which the crypt() of libxcrypt accepts. Go writes
// $2a$, which is the same algorithm for passwords of valid UTF-8 (ValidPassword).
func hashBcrypt(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	if err != nil {
		return "", err
	}
	return "{BLF-CRYPT}$2y$" + strings.TrimPrefix(string(b), "$2a$"), nil
}

// Where everything lives. The directory belongs to root; the lists in it are read by Postfix and Dovecot, which never write to it.
const (
	Dir       = "/etc/mpn/mail"
	VDomains  = Dir + "/vdomains" // the domains this machine receives mail for
	VMailbox  = Dir + "/vmailbox" // address -> maildir, for Postfix
	VAlias    = Dir + "/valias"   // address -> addresses, for Postfix
	UsersFile = Dir + "/users"    // address:hash, for Dovecot
	DKIMDir   = Dir + "/dkim"     // one folder per domain, with the private key
	MailRoot  = "/var/vmail"      // the maildirs
	MailUser  = "vmail"
)

// Limits. They keep a request small and a lookup table small enough for a 1 GB machine.
const (
	MaxDomains   = 50
	MaxMailboxes = 500
	MaxAliases   = 500
	MaxQuotaMB   = 102400
)

var (
	labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	localRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._+-]{0,63}$`)
	selRe   = regexp.MustCompile(`^[a-z0-9]{1,24}$`)
	// hashRe is the only shape of password hash that may reach the Dovecot user list; it has no colon, no space and no newline, so a
	// stored value can never add a field or a line.
	hashRe = regexp.MustCompile(`^\{ARGON2ID\}\$argon2id\$v=19\$m=\d{4,6},t=\d{1,2},p=\d{1}\$[A-Za-z0-9+/]{16,64}\$[A-Za-z0-9+/]{32,128}$`)
	// bcryptRe is the hash used on the RHEL family, whose Dovecot is built without argon2 ("Unknown scheme ARGON2ID", seen on Rocky
	// Linux 10 and AlmaLinux 10); bcrypt is in its list (doveadm pw -l) through the crypt() of libxcrypt.
	bcryptRe = regexp.MustCompile(`^\{BLF-CRYPT\}\$2y\$1[0-4]\$[./A-Za-z0-9]{53}$`)
)

// ValidDomain accepts a lower-case host name of two or more labels, as a mail domain or the name of the mail host.
func ValidDomain(d string) error {
	if len(d) < 4 || len(d) > 253 || strings.Count(d, ".") < 1 {
		return fmt.Errorf("%q is not a domain name", d)
	}
	for _, l := range strings.Split(d, ".") {
		if !labelRe.MatchString(l) {
			return fmt.Errorf("%q is not a domain name (lower-case letters, digits and hyphens between dots)", d)
		}
	}
	if last := d[strings.LastIndex(d, ".")+1:]; len(last) < 2 || strings.Trim(last, "0123456789") == "" {
		return fmt.Errorf("%q does not end in a top-level domain", d)
	}
	return nil
}

// ValidLocal accepts the part of an address before the @.
func ValidLocal(l string) error {
	if !localRe.MatchString(l) || strings.Contains(l, "..") || strings.HasSuffix(l, ".") {
		return fmt.Errorf("%q cannot be the first part of an address (lower-case letters, digits and . _ + -)", l)
	}
	return nil
}

// SplitAddress checks an address and returns its two parts.
func SplitAddress(a string) (local, domain string, err error) {
	local, domain, ok := strings.Cut(a, "@")
	if !ok || strings.Contains(domain, "@") {
		return "", "", fmt.Errorf("%q is not an address", a)
	}
	if err := ValidLocal(local); err != nil {
		return "", "", err
	}
	if err := ValidDomain(domain); err != nil {
		return "", "", err
	}
	return local, domain, nil
}

// ValidSelector accepts the name of a DKIM key: letters and digits only, since it becomes a label of a DNS name and part of a path.
func ValidSelector(s string) error {
	if !selRe.MatchString(s) {
		return fmt.Errorf("%q is not a key name (up to 24 lower-case letters and digits)", s)
	}
	return nil
}

// ValidQuota accepts a size in megabytes; 0 means no limit.
func ValidQuota(mb int) error {
	if mb < 0 || mb > MaxQuotaMB {
		return fmt.Errorf("the size of a mailbox is 0 (no limit) to %d MB", MaxQuotaMB)
	}
	return nil
}

// ValidPassword accepts a password a person will type: 10 to 128 characters, valid UTF-8, no control character.
func ValidPassword(pw string) error {
	if n := utf8.RuneCountInString(pw); n < 10 || n > 128 || !utf8.ValidString(pw) {
		return fmt.Errorf("the password must have 10 to 128 characters")
	}
	for _, r := range pw {
		if unicode.IsControl(r) {
			return fmt.Errorf("the password cannot contain control characters")
		}
	}
	return nil
}

// ValidHash reports whether h is a hash made by HashPassword, on either family.
func ValidHash(h string) bool { return hashRe.MatchString(h) || bcryptRe.MatchString(h) }

// bcryptCost keeps one login near 50 ms on one small core.
const bcryptCost = 10

// The cost of one login. Dovecot checks a password with this much memory, so it is the OWASP minimum, not the strongest setting: on a
// 1 GB machine several people signing in at once must not exhaust it.
const (
	argonMem     = 19456 // KiB
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
)

var b64 = base64.RawStdEncoding

// HashPassword returns the hash of pw in the form Dovecot reads: {ARGON2ID}, then the standard encoded string of argon2id, on Debian
// and Ubuntu; {BLF-CRYPT} and bcrypt on the RHEL family (see bcryptRe).
func HashPassword(pw string) (string, error) {
	if err := ValidPassword(pw); err != nil {
		return "", err
	}
	if distro.IsEL() {
		return hashBcrypt(pw)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMem, argonThreads, argonKeyLen)
	return fmt.Sprintf("{ARGON2ID}$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMem, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// NewDKIMKey makes the key pair of a domain. private is a PEM file for OpenDKIM; txt is the value of the DNS record to publish.
func NewDKIMKey() (private, txt string, err error) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	pub, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		return "", "", err
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	return string(pemKey), "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(pub), nil
}

// DKIMTextFromPrivate gives the DNS value again from a stored private key, so the page can show the record at any time.
func DKIMTextFromPrivate(privatePEM string) (string, error) {
	blk, _ := pem.Decode([]byte(privatePEM))
	if blk == nil {
		return "", fmt.Errorf("the key file is not PEM")
	}
	k, err := x509.ParsePKCS1PrivateKey(blk.Bytes)
	if err != nil {
		return "", err
	}
	pub, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		return "", err
	}
	return "v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(pub), nil
}

// Record is one DNS record a person has to add where the domain is managed.
type Record struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
}

// Records are the records mail needs for domain: where mail goes (MX), who may send it (SPF), the key that signs it (DKIM) and what a
// receiver should do with mail that fails both (DMARC). host is the name of this machine as a mail server; it needs an address record
// of its own and a reverse name, which the page explains because they are set where the address is rented, not at the domain.
func Records(domain, host, selector, dkimTXT string) []Record {
	return []Record{
		{Type: "MX", Name: domain, Value: "10 " + host + "."},
		{Type: "TXT", Name: domain, Value: "v=spf1 mx -all", Note: "spf"},
		{Type: "TXT", Name: selector + "._domainkey." + domain, Value: dkimTXT, Note: "dkim"},
		{Type: "TXT", Name: "_dmarc." + domain, Value: "v=DMARC1; p=none; adkim=s; aspf=s; rua=mailto:postmaster@" + domain, Note: "dmarc"},
	}
}
