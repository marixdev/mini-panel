package webauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mpn/internal/site"
)

// Role is what a delegated user may do on one website. The roles are cumulative: each one includes the ones before it.
type Role string

const (
	RoleView   Role = "view"   // read-only pages of the website
	RoleManage Role = "manage" // + changes to files, cron, databases, backups (not restore), mail, WordPress, PHP settings
	RoleFull   Role = "full"   // + terminal, SSH, SSL, domain, vhost, restore, remote database access, cloud backups of the site
)

// rank orders the roles. The empty role and any unknown one rank 0, so they grant nothing.
func (r Role) rank() int {
	switch r {
	case RoleView:
		return 1
	case RoleManage:
		return 2
	case RoleFull:
		return 3
	}
	return 0
}

// Valid reports whether r is one of the three roles.
func (r Role) Valid() bool { return r.rank() > 0 }

// AtLeast reports whether r includes everything min allows.
func (r Role) AtLeast(min Role) bool { return r.rank() >= min.rank() && min.rank() > 0 }

// MaxUsers is how many delegated users a panel keeps.
const MaxUsers = 50

// User is a delegated account: it signs in to the panel and can work only on the websites it is given, each with a role.
type User struct {
	Name     string          `json:"name"`
	Hash     string          `json:"hash"`
	Disabled bool            `json:"disabled,omitempty"`
	Sites    map[string]Role `json:"sites"`
	TOTP     string          `json:"totp,omitempty"`
	TOTPStep int64           `json:"totp_step,omitempty"`
	Created  int64           `json:"created,omitempty"`
	// PMA lets the user open the database browser for a database of a website they manage. Only the administrator sets it.
	PMA bool `json:"pma,omitempty"`
}

// Account returns the password side of u in the form Account.Check takes, so the two-step and password checks are shared.
func (u User) Account() Account {
	return Account{User: u.Name, Hash: u.Hash, TOTP: u.TOTP, TOTPStep: u.TOTPStep}
}

// Check reports whether name and pw are right for this user, with the same timing as the administrator's check.
func (u User) Check(name, pw string) bool { return u.Account().Check(name, pw) }

// RoleOn returns the role u has on a website, or "" when none.
func (u User) RoleOn(siteName string) Role { return u.Sites[siteName] }

var userNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{2,31}$`)

// ValidUserName reports whether name may be a delegated user's name: lower case, starts with a letter, 3 to 32 characters.
func ValidUserName(name string) bool { return userNameRe.MatchString(name) }

// ValidateUsers checks a whole list: names, duplicates, the administrator's name, roles, site names and the count. It is what
// LoadUsers and the admin page both apply before a list is used or saved.
func ValidateUsers(users []User, admin string) error {
	if len(users) > MaxUsers {
		return fmt.Errorf("more than %d delegated users", MaxUsers)
	}
	seen := map[string]bool{}
	for _, u := range users {
		if !ValidUserName(u.Name) {
			return fmt.Errorf("user name %q is not valid", u.Name)
		}
		low := strings.ToLower(u.Name)
		if low == strings.ToLower(admin) {
			return fmt.Errorf("user name %q is the administrator's", u.Name)
		}
		if seen[low] {
			return fmt.Errorf("user name %q is listed twice", u.Name)
		}
		seen[low] = true
		if u.Hash == "" {
			return fmt.Errorf("user %q has no password", u.Name)
		}
		for s, r := range u.Sites {
			if !site.ValidName(s) {
				return fmt.Errorf("user %q: site name %q is not valid", u.Name, s)
			}
			if !r.Valid() {
				return fmt.Errorf("user %q: role %q is not valid", u.Name, r)
			}
		}
	}
	return nil
}

// LoadUsers reads the delegated users. A missing file is no users. A damaged or invalid file is an error, and the caller
// then lets no delegated user sign in; the administrator is not affected.
func LoadUsers(path, admin string) ([]User, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var users []User
	if err := json.Unmarshal(b, &users); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := ValidateUsers(users, admin); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return users, nil
}

// SaveUsers writes the list through a temporary file, readable by its owner only, the way SaveAccount writes the account. The
// list is validated first, so a bad list is never written.
func SaveUsers(path string, users []User, admin string, uid, gid int) error {
	if err := ValidateUsers(users, admin); err != nil {
		return err
	}
	if users == nil {
		users = []User{}
	}
	b, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".users-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && !isWindows() {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if uid >= 0 {
		if err := os.Chown(tmp.Name(), uid, gid); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), path)
}
