// Package access switches a site's system account between "files only" (the default: no login) and "can log in over
// SSH or SFTP with a password". It only ever touches an account that carries this site's mark, so it can never change
// the login of a system account, an administrator, or another site's user.
package access

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"mpn/internal/dbadmin"
	"mpn/internal/provision"
	"mpn/internal/site"
)

const (
	LoginShell = "/bin/bash"
	NoShell    = "/usr/sbin/nologin"
)

// Runner runs a program with the given standard input and returns what it printed. The password travels on standard
// input, never on a command line where any user of the machine could read it.
type Runner interface {
	Run(stdin, name string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(stdin, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// owned checks that the account exists and was made by mpn for this very site.
func owned(r Runner, s site.Site) (home string, err error) {
	out, err := r.Run("", "getent", "passwd", s.User)
	if err != nil {
		return "", fmt.Errorf("account %s does not exist; apply the site first", s.User)
	}
	f := strings.Split(strings.TrimSpace(out), ":")
	if len(f) < 7 || f[4] != provision.AccountMark(s.Name) {
		return "", fmt.Errorf("account %s was not created by mpn for this site, so its login is left alone", s.User)
	}
	return f[5], nil
}

// bannerScript runs as the site's own user, never as root, so it can only touch what that user owns. It silences the
// system's welcome text for this account (on Ubuntu that is the advertisement block and the "Last login" line) and adds
// a short note of our own to the end of its login profile, once. $1 is the home directory.
const bannerScript = `home="$1"; umask 077
: > "$home/.hushlogin"
grep -q '^# mpn banner' "$home/.profile" 2>/dev/null && exit 0
cat >> "$home/.profile"`

// bannerSnippet is the text added to the profile. It is shown only to an interactive login, so scp, sftp and commands
// run over ssh stay silent. The host name and the web root were checked by site.Validate and contain no quote.
func bannerSnippet(s site.Site) string {
	return fmt.Sprintf(`
# mpn banner: replaces the system's welcome text for this account
case $- in *i*)
  if [ -z "$MPN_BANNER" ]; then
    MPN_BANNER=1; export MPN_BANNER
    printf '\n  Mini Panel · %%s\n  Account %%s · web root %%s\n  Type exit to leave.\n\n' '%s' "$USER" '%s'
  fi;;
esac
`, s.Hostnames[0], s.Root)
}

// WelcomeText gives the account a short welcome of its own in place of the system's. It is best effort: the login
// works without it, so a failure here is not an error for the caller.
func WelcomeText(r Runner, s site.Site, home string) error {
	if !strings.HasPrefix(home, "/home/") || !site.SafePath(home) || len(s.Hostnames) == 0 {
		return fmt.Errorf("unexpected home directory %q", home)
	}
	out, err := r.Run(bannerSnippet(s), "runuser", "-u", s.User, "--", "sh", "-c", bannerScript, "sh", home)
	if err != nil {
		return fmt.Errorf("welcome text: %s", strings.TrimSpace(out))
	}
	return nil
}

// Enable lets the site's account log in with the given password.
func Enable(r Runner, s site.Site, password string) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if !dbadmin.ValidPassword(password) {
		return errors.New("invalid password: 12 to 64 printable characters without space, quotes, backslash or backtick")
	}
	home, err := owned(r, s)
	if err != nil {
		return err
	}
	// The error text of these programs is dropped on purpose: it could repeat the line that was sent to them.
	if _, err := r.Run(s.User+":"+password+"\n", "chpasswd"); err != nil {
		return errors.New("setting the password failed")
	}
	if out, err := r.Run("", "usermod", "-s", LoginShell, s.User); err != nil {
		return fmt.Errorf("usermod: %s", strings.TrimSpace(out))
	}
	_ = WelcomeText(r, s, home)
	return nil
}

// Disable takes the login away again: no shell, the password is locked, and the panel's key is removed, since the
// terminal and the file manager of the panel need a login to work.
func Disable(r Runner, s site.Site) error {
	if err := s.Validate(); err != nil {
		return err
	}
	home, err := owned(r, s)
	if err != nil {
		return err
	}
	if out, err := r.Run("", "usermod", "-s", NoShell, s.User); err != nil {
		return fmt.Errorf("usermod: %s", strings.TrimSpace(out))
	}
	if out, err := r.Run("", "passwd", "-l", s.User); err != nil {
		return fmt.Errorf("passwd: %s", strings.TrimSpace(out))
	}
	_ = editPanelKey(r, s, home, "remove", "")
	return nil
}

// Enabled reports from a passwd file whether the user has a login shell.
func Enabled(passwdFile, user string) bool {
	b, err := os.ReadFile(passwdFile)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, ":")
		if len(f) >= 7 && f[0] == user {
			sh := strings.TrimSpace(f[6])
			return sh != "" && !strings.HasSuffix(sh, "/nologin") && !strings.HasSuffix(sh, "/false")
		}
	}
	return false
}

// panelLine is the only text that may be put into an account's authorized_keys for the panel. It is the output of
// panelkey.AuthorizedLine; checking it again here keeps this package safe whoever calls it.
var panelLine = regexp.MustCompile(`^restrict,pty,from="127\.0\.0\.1,::1" ssh-ed25519 [A-Za-z0-9+/]+={0,2} mpn-panel$`)

// keyScript runs as the site's own user, never as root. $1 is the home directory and $2 is "install" or "remove".
// It drops an older line of the panel from authorized_keys and, for "install", adds the current one from standard
// input. Lines the user put there themselves are kept.
const keyScript = `home="$1"; mode="$2"; umask 077
mkdir -p "$home/.ssh" && chmod 700 "$home/.ssh" || exit 1
f="$home/.ssh/authorized_keys"
{ grep -v ' mpn-panel$' "$f" 2>/dev/null || true; if [ "$mode" = install ]; then cat; fi; } > "$f.new" && mv "$f.new" "$f"`

func editPanelKey(r Runner, s site.Site, home, mode, line string) error {
	if !strings.HasPrefix(home, "/home/") || !site.SafePath(home) {
		return fmt.Errorf("unexpected home directory %q", home)
	}
	stdin := ""
	if mode == "install" {
		stdin = line + "\n"
	}
	if out, err := r.Run(stdin, "runuser", "-u", s.User, "--", "sh", "-c", keyScript, "sh", home, mode); err != nil {
		return fmt.Errorf("authorized_keys: %s", strings.TrimSpace(out))
	}
	return nil
}

// InstallPanelKey lets the panel log in as the site's account. Calling it again replaces the panel's line, so it also
// repairs an account whose line was deleted. Only an account that mpn made for this site is touched.
func InstallPanelKey(r Runner, s site.Site, line string) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if !panelLine.MatchString(line) {
		return errors.New("the panel's key line is not in the expected form")
	}
	home, err := owned(r, s)
	if err != nil {
		return err
	}
	return editPanelKey(r, s, home, "install", line)
}

// RemovePanelKey takes the panel's key out of the account's authorized_keys.
func RemovePanelKey(r Runner, s site.Site) error {
	if err := s.Validate(); err != nil {
		return err
	}
	home, err := owned(r, s)
	if err != nil {
		return err
	}
	return editPanelKey(r, s, home, "remove", "")
}
