package dbadmin

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Temporary users are what the database browser (phpMyAdmin) logs in with. The panel never keeps the password of a
// database. When somebody opens the browser for one database, a user is made for that visit alone: it can reach that one
// database and nothing else, it has a password nobody has seen, and it is dropped when the visit ends or its time is up.
//
// The name carries the moment it expires, "pma_<seconds since 1970 in base 36>_<8 random characters>", so that a sweep
// needs no list of its own: it reads the names from the server and drops the ones whose time has passed. That also works
// after a restart, a crash, or a user dropped half way.

// TempTTL is how long a visit may last.
const TempTTL = 30 * time.Minute

var tempRe = regexp.MustCompile(`^pma_[0-9a-z]{6,8}_[0-9a-z]{8}$`)

// ValidTempName reports whether s has the shape of a temporary user. Only such names are ever dropped by the panel, so
// a request cannot make it drop a real user.
func ValidTempName(s string) bool { return tempRe.MatchString(s) }

// TempExpiry reads the moment a temporary user expires out of its name.
func TempExpiry(name string) (time.Time, bool) {
	if !ValidTempName(name) {
		return time.Time{}, false
	}
	secs, err := strconv.ParseInt(strings.Split(name, "_")[1], 36, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(secs, 0), true
}

const tempAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func newTempName(expires time.Time) (string, error) {
	max := big.NewInt(int64(len(tempAlphabet)))
	r := make([]byte, 8)
	for i := range r {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		r[i] = tempAlphabet[n.Int64()]
	}
	return "pma_" + strconv.FormatInt(expires.Unix(), 36) + "_" + string(r), nil
}

// TempUser is what a visit logs in with.
type TempUser struct {
	Name, Password string
	Expires        time.Time
}

func tempCreateSQL(user, pw, db string) string {
	return fmt.Sprintf("CREATE USER '%[1]s'@'localhost' IDENTIFIED BY '%[2]s' WITH MAX_USER_CONNECTIONS 6;\n"+
		"GRANT ALL PRIVILEGES ON `%[3]s`.* TO '%[1]s'@'localhost';\n", user, pw, escapeGrantDB(db))
}

func tempDropSQL(user string) string {
	return fmt.Sprintf("DROP USER IF EXISTS '%s'@'localhost';\n", user)
}

// CreateTemp makes a temporary user for one database. The caller has already checked that the database is one the panel
// manages; this only checks that the name is a safe one.
func (a Admin) CreateTemp(db string, now time.Time) (TempUser, error) {
	if !ValidName(db) {
		return TempUser{}, fmt.Errorf("invalid database name %q", db)
	}
	used, err := a.InUse(db)
	if err != nil {
		return TempUser{}, err
	}
	if !used {
		return TempUser{}, fmt.Errorf("no database named %q on the server", db)
	}
	exp := now.Add(TempTTL)
	name, err := newTempName(exp)
	if err != nil {
		return TempUser{}, err
	}
	pw, err := NewPassword()
	if err != nil {
		return TempUser{}, err
	}
	if _, err := a.R.Run(tempCreateSQL(name, pw, db)); err != nil {
		_, _ = a.R.Run(tempDropSQL(name))
		return TempUser{}, redact(err, pw)
	}
	return TempUser{Name: name, Password: pw, Expires: exp}, nil
}

// DropTemp ends a visit. Anything that is not shaped like a temporary user is refused, whatever else it is.
func (a Admin) DropTemp(name string) error {
	if !ValidTempName(name) {
		return fmt.Errorf("%q is not a temporary user", name)
	}
	_, err := a.R.Run(tempDropSQL(name))
	return err
}

// CleanTemp drops every temporary user whose time has passed and returns their names. A name that merely starts with
// "pma_" but is not shaped like a temporary user is left alone: it belongs to somebody else.
func (a Admin) CleanTemp(now time.Time) ([]string, error) {
	out, err := a.R.Run("SELECT DISTINCT user FROM mysql.user WHERE user LIKE 'pma\\_%';\n")
	if err != nil {
		return nil, err
	}
	var dropped []string
	var first error
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		exp, ok := TempExpiry(name)
		if !ok || exp.After(now) {
			continue
		}
		if err := a.DropTemp(name); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		dropped = append(dropped, name)
	}
	return dropped, errors.Join(first)
}
