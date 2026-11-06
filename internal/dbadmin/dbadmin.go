// Package dbadmin creates, re-passwords and drops per-site databases and their users on the local MySQL or MariaDB
// server. Every database gets one user with the same name, who can reach that database and nothing else.
package dbadmin

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os/exec"
	"regexp"
	"strings"
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,31}$`)

// ValidName reports whether s is a safe database and user name: lower case letters, digits and underscore, at most
// 32 characters (MySQL's limit for user names). Names are always quoted in SQL, and this check is what keeps them
// from carrying SQL of their own.
func ValidName(s string) bool { return nameRe.MatchString(s) }

// BaseName turns a site name into the default database name for it ("my-blog" becomes "my_blog").
func BaseName(site string) string { return strings.ReplaceAll(site, "-", "_") }

// OwnedBy reports whether a database name may belong to a site: it is the site's base name, or starts with it
// followed by an underscore. That keeps names recognizable and apart from other sites'.
func OwnedBy(name, site string) bool {
	b := BaseName(site)
	return name == b || strings.HasPrefix(name, b+"_")
}

// passwordAlphabet leaves out characters that are easy to confuse (0 O 1 l I).
const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

// NewPassword returns 24 random characters, about 140 bits.
func NewPassword() (string, error) {
	max := big.NewInt(int64(len(passwordAlphabet)))
	out := make([]byte, 24)
	for i := range out {
		n, err := rand.Int(rand.Reader, max) // rand.Int avoids the modulo bias of a plain remainder
		if err != nil {
			return "", err
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}

// ValidPassword accepts 12 to 64 printable ASCII characters other than space, quotes, backslash and backtick, the
// characters that would need escaping inside an SQL string.
func ValidPassword(p string) bool {
	if len(p) < 12 || len(p) > 64 {
		return false
	}
	for _, c := range []byte(p) {
		if c < 0x21 || c > 0x7e || c == '\'' || c == '"' || c == '\\' || c == '`' {
			return false
		}
	}
	return true
}

// escapeGrantDB escapes the underscores of a database name for GRANT. In GRANT the database name is a pattern in which
// "_" matches any single character, so an unescaped "my_blog" would also give access to "myXblog".
func escapeGrantDB(name string) string { return strings.ReplaceAll(name, "_", `\_`) }

func createSQL(name, pw string) string {
	return fmt.Sprintf("CREATE DATABASE `%[1]s` CHARACTER SET utf8mb4;\n"+
		"CREATE USER '%[1]s'@'localhost' IDENTIFIED BY '%[2]s';\n"+
		"GRANT ALL PRIVILEGES ON `%[3]s`.* TO '%[1]s'@'localhost';\n", name, pw, escapeGrantDB(name))
}

func passwordSQL(name, pw string) string {
	return fmt.Sprintf("ALTER USER '%s'@'localhost' IDENTIFIED BY '%s';\n", name, pw)
}

func dropSQL(name string) string {
	return fmt.Sprintf("DROP DATABASE IF EXISTS `%[1]s`;\nDROP USER IF EXISTS '%[1]s'@'localhost';\n", name)
}

func inUseSQL(name string) string {
	return fmt.Sprintf("SELECT (SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = '%[1]s') + "+
		"(SELECT COUNT(*) FROM mysql.user WHERE user = '%[1]s');\n", name)
}

// Runner sends SQL to the server and returns what it printed.
type Runner interface {
	Run(sql string) (string, error)
}

// ExecRunner runs the command-line client as the current user, normally root, who reaches the server over its socket.
// The SQL goes in on stdin so that a password never shows up in the process list.
type ExecRunner struct{ Client string }

func (r ExecRunner) Run(sql string) (string, error) {
	cmd := exec.Command(r.Client, "--batch", "--skip-column-names")
	cmd.Stdin = strings.NewReader(sql)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %s", r.Client, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// Detect returns the client program to use: mariadb if installed, otherwise mysql.
func Detect() (string, error) {
	for _, c := range []string{"mariadb", "mysql"} {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no mariadb or mysql client found; install a database engine first (mpn plan -engine ...)")
}

// DetectDump returns the dump program that matches the installed engine: mariadb-dump, otherwise mysqldump.
func DetectDump() (string, error) {
	for _, c := range []string{"mariadb-dump", "mysqldump"} {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no mariadb-dump or mysqldump found; install a database engine first (mpn plan -engine ...)")
}

type Admin struct{ R Runner }

// redact keeps a password out of an error message; some server errors quote the text near the problem.
func redact(err error, pw string) error {
	if err == nil || pw == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), pw, "********"))
}

// InUse reports whether a database or a user with this name already exists on the server.
func (a Admin) InUse(name string) (bool, error) {
	if !ValidName(name) {
		return false, fmt.Errorf("invalid name %q", name)
	}
	out, err := a.R.Run(inUseSQL(name))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "0", nil
}

// Create makes the database and its user. It refuses a name that already exists on the server, so it never takes over
// a database it did not create, and it removes what it made if a later statement fails.
func (a Admin) Create(name, password string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid name %q: use a-z, 0-9 and _, at most 32 characters", name)
	}
	if !ValidPassword(password) {
		return errors.New("invalid password: 12 to 64 printable characters without space, quotes, backslash or backtick")
	}
	used, err := a.InUse(name)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("a database or user named %q already exists on the server", name)
	}
	if _, err := a.R.Run(createSQL(name, password)); err != nil {
		_, _ = a.R.Run(dropSQL(name))
		return redact(err, password)
	}
	return nil
}

// SetPassword gives an existing user a new password. The old one stops working at once.
func (a Admin) SetPassword(name, password string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid name %q", name)
	}
	if !ValidPassword(password) {
		return errors.New("invalid password: 12 to 64 printable characters without space, quotes, backslash or backtick")
	}
	used, err := a.InUse(name)
	if err != nil {
		return err
	}
	if !used {
		return fmt.Errorf("no database or user named %q on the server", name)
	}
	_, err = a.R.Run(passwordSQL(name, password))
	return redact(err, password)
}

// Drop deletes the database and its user. It is not an error if either is already gone.
func (a Admin) Drop(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid name %q", name)
	}
	_, err := a.R.Run(dropSQL(name))
	return err
}
