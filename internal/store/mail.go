package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"mpn/internal/mail"
)

const (
	mailKey     = "mail"      // "on" or "off"
	mailHostKey = "mail_host" // the name of this machine as a mail server
)

// MailOn says whether the person turned mail on.
func (s *Store) MailOn() (bool, error) {
	v, err := s.Setting(mailKey, "off")
	return v == "on", err
}

// SetMailOn records that mail is on or off.
func (s *Store) SetMailOn(on bool) error {
	if on {
		return s.SetSetting(mailKey, "on")
	}
	return s.SetSetting(mailKey, "off")
}

// MailHost is the name of this machine as a mail server, or "" before the person chose one.
func (s *Store) MailHost() (string, error) { return s.Setting(mailHostKey, "") }

// SetMailHost stores the mail host after checking it.
func (s *Store) SetMailHost(host string) error {
	if err := mail.ValidDomain(host); err != nil {
		return err
	}
	return s.SetSetting(mailHostKey, host)
}

// MailDomain is a domain mail is received for.
type MailDomain struct {
	Name      string
	HasKey    bool
	Mailboxes int
	Aliases   int
}

// MailDomains lists the domains with how many mailboxes and aliases each has.
func (s *Store) MailDomains() ([]MailDomain, error) {
	rows, err := s.db.Query(`SELECT d.name, d.dkim_key <> '',
        (SELECT count(*) FROM mailboxes m WHERE m.domain = d.name),
        (SELECT count(*) FROM mail_aliases a WHERE a.domain = d.name)
        FROM mail_domains d ORDER BY d.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MailDomain{}
	for rows.Next() {
		var d MailDomain
		if err := rows.Scan(&d.Name, &d.HasKey, &d.Mailboxes, &d.Aliases); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AddMailDomain stores a domain with its DKIM key. The same domain twice is refused.
func (s *Store) AddMailDomain(name, dkimKey string) error {
	if err := mail.ValidDomain(name); err != nil {
		return err
	}
	if dkimKey != "" {
		if _, err := mail.DKIMTextFromPrivate(dkimKey); err != nil {
			return fmt.Errorf("DKIM key: %w", err)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM mail_domains`).Scan(&n); err != nil {
		return err
	}
	if n >= mail.MaxDomains {
		return fmt.Errorf("at most %d mail domains", mail.MaxDomains)
	}
	if _, err := tx.Exec(`INSERT INTO mail_domains (name, dkim_key, created_at) VALUES (?,?,?)`, name, dkimKey, time.Now().Unix()); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return existsError{"mail domain"}
		}
		return err
	}
	return tx.Commit()
}

// DeleteMailDomain removes a domain with its mailboxes and aliases. The stored mail on disk is the caller's concern.
func (s *Store) DeleteMailDomain(name string) error {
	res, err := s.db.Exec(`DELETE FROM mail_domains WHERE name = ?`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("mail domain %s: %w", name, ErrNotFound)
	}
	return nil
}

// MailDKIMKey returns the private DKIM key of a domain.
func (s *Store) MailDKIMKey(domain string) (string, error) {
	var k string
	err := s.db.QueryRow(`SELECT dkim_key FROM mail_domains WHERE name = ?`, domain).Scan(&k)
	if err != nil {
		return "", fmt.Errorf("mail domain %s: %w", domain, ErrNotFound)
	}
	return k, nil
}

// SetMailDKIMKey replaces the key of a domain.
func (s *Store) SetMailDKIMKey(domain, key string) error {
	if _, err := mail.DKIMTextFromPrivate(key); err != nil {
		return fmt.Errorf("DKIM key: %w", err)
	}
	res, err := s.db.Exec(`UPDATE mail_domains SET dkim_key = ? WHERE name = ?`, key, domain)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("mail domain %s: %w", domain, ErrNotFound)
	}
	return nil
}

// Mailbox is a mailbox as a page shows it; the password hash never leaves the store this way.
type Mailbox struct {
	Address string
	QuotaMB int
}

// Mailboxes lists the mailboxes of one domain, or of all when domain is empty.
func (s *Store) Mailboxes(domain string) ([]Mailbox, error) {
	q, args := `SELECT address, quota_mb FROM mailboxes ORDER BY address`, []any{}
	if domain != "" {
		q, args = `SELECT address, quota_mb FROM mailboxes WHERE domain = ? ORDER BY address`, []any{domain}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Mailbox{}
	for rows.Next() {
		var m Mailbox
		if err := rows.Scan(&m.Address, &m.QuotaMB); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMailbox stores a mailbox. hash must be made by mail.HashPassword. The domain must exist, and an address that is already a mailbox
// or an alias is refused.
func (s *Store) AddMailbox(address, hash string, quotaMB int) error {
	_, domain, err := mail.SplitAddress(address)
	if err != nil {
		return err
	}
	if !mail.ValidHash(hash) {
		return fmt.Errorf("the password is not a hash made by mpn")
	}
	if err := mail.ValidQuota(quotaMB); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM mail_domains WHERE name = ?`, domain).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("mail domain %s: %w", domain, ErrNotFound)
	}
	if err := tx.QueryRow(`SELECT count(*) FROM mailboxes`).Scan(&n); err != nil {
		return err
	}
	if n >= mail.MaxMailboxes {
		return fmt.Errorf("at most %d mailboxes", mail.MaxMailboxes)
	}
	if err := tx.QueryRow(`SELECT count(*) FROM mail_aliases WHERE address = ?`, address).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return existsError{"address (it is an alias)"}
	}
	if _, err := tx.Exec(`INSERT INTO mailboxes (address, domain, hash, quota_mb, created_at) VALUES (?,?,?,?,?)`, address, domain, hash, quotaMB, time.Now().Unix()); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return existsError{"mailbox"}
		}
		return err
	}
	return tx.Commit()
}

// DeleteMailbox removes a mailbox.
func (s *Store) DeleteMailbox(address string) error {
	res, err := s.db.Exec(`DELETE FROM mailboxes WHERE address = ?`, address)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("mailbox %s: %w", address, ErrNotFound)
	}
	return nil
}

// SetMailboxPassword replaces the password hash of a mailbox.
func (s *Store) SetMailboxPassword(address, hash string) error {
	if !mail.ValidHash(hash) {
		return fmt.Errorf("the password is not a hash made by mpn")
	}
	res, err := s.db.Exec(`UPDATE mailboxes SET hash = ? WHERE address = ?`, hash, address)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("mailbox %s: %w", address, ErrNotFound)
	}
	return nil
}

// SetMailboxQuota changes the size limit of a mailbox; 0 removes it.
func (s *Store) SetMailboxQuota(address string, quotaMB int) error {
	if err := mail.ValidQuota(quotaMB); err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE mailboxes SET quota_mb = ? WHERE address = ?`, quotaMB, address)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("mailbox %s: %w", address, ErrNotFound)
	}
	return nil
}

// Alias is an address and the addresses its mail goes to.
type Alias struct {
	Address string
	Targets []string
}

// MailAliases lists the aliases of one domain, or of all when domain is empty.
func (s *Store) MailAliases(domain string) ([]Alias, error) {
	q, args := `SELECT address, targets FROM mail_aliases ORDER BY address`, []any{}
	if domain != "" {
		q, args = `SELECT address, targets FROM mail_aliases WHERE domain = ? ORDER BY address`, []any{domain}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alias{}
	for rows.Next() {
		var a Alias
		var t string
		if err := rows.Scan(&a.Address, &t); err != nil {
			return nil, err
		}
		a.Targets = strings.Split(t, ",")
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetMailAlias creates an alias or replaces its targets. An address that is a mailbox is refused: mail for it would never reach the
// aliases.
func (s *Store) SetMailAlias(address string, targets []string) error {
	_, domain, err := mail.SplitAddress(address)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var clean []string
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		if _, _, err := mail.SplitAddress(t); err != nil {
			return err
		}
		seen[t] = true
		clean = append(clean, t)
	}
	if len(clean) == 0 {
		return fmt.Errorf("an alias needs at least one target")
	}
	if len(clean) > 20 {
		return fmt.Errorf("an alias has at most 20 targets")
	}
	sort.Strings(clean)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM mail_domains WHERE name = ?`, domain).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("mail domain %s: %w", domain, ErrNotFound)
	}
	if err := tx.QueryRow(`SELECT count(*) FROM mailboxes WHERE address = ?`, address).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return existsError{"address (it is a mailbox)"}
	}
	if err := tx.QueryRow(`SELECT count(*) FROM mail_aliases WHERE address <> ?`, address).Scan(&n); err != nil {
		return err
	}
	if n >= mail.MaxAliases {
		return fmt.Errorf("at most %d aliases", mail.MaxAliases)
	}
	if _, err := tx.Exec(`INSERT INTO mail_aliases (address, domain, targets, created_at) VALUES (?,?,?,?)
        ON CONFLICT(address) DO UPDATE SET targets = excluded.targets`, address, domain, strings.Join(clean, ","), time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteMailAlias removes an alias.
func (s *Store) DeleteMailAlias(address string) error {
	res, err := s.db.Exec(`DELETE FROM mail_aliases WHERE address = ?`, address)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("alias %s: %w", address, ErrNotFound)
	}
	return nil
}

// MailState gathers everything the mail files are made from. The host comes from the settings.
func (s *Store) MailState() (mail.State, error) {
	host, err := s.MailHost()
	if err != nil {
		return mail.State{}, err
	}
	st := mail.State{Host: host, Aliases: map[string][]string{}, Keys: map[string]string{}}
	rows, err := s.db.Query(`SELECT name, dkim_key FROM mail_domains ORDER BY name`)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var d, k string
		if err := rows.Scan(&d, &k); err != nil {
			rows.Close()
			return st, err
		}
		st.Domains = append(st.Domains, d)
		if k != "" {
			st.Keys[d] = k
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return st, err
	}
	rows.Close()
	urows, err := s.db.Query(`SELECT address, hash, quota_mb FROM mailboxes ORDER BY address`)
	if err != nil {
		return st, err
	}
	defer urows.Close()
	for urows.Next() {
		var u mail.User
		if err := urows.Scan(&u.Address, &u.Hash, &u.QuotaMB); err != nil {
			return st, err
		}
		st.Users = append(st.Users, u)
	}
	if err := urows.Err(); err != nil {
		return st, err
	}
	urows.Close()
	al, err := s.MailAliases("")
	if err != nil {
		return st, err
	}
	for _, a := range al {
		st.Aliases[a.Address] = a.Targets
	}
	return st, nil
}
