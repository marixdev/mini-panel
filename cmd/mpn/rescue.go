package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/term"

	"mpn/internal/security"
	"mpn/internal/webauth"
)

// The rescue menu is what "mpn" with no arguments shows on a terminal: the commands for getting back into a locked-out panel or
// server, so that nobody has to remember them in a hurry. Each entry runs the mpn command it names, the same program with the same
// checks, and prints that command first so it can be typed next time. Without a terminal (a script, cron, a pipe) mpn prints its
// usage as before.

// rescueIO is where the menu reads and writes, and how it runs a command and reads a password; the tests replace all of it.
type rescueIO struct {
	in       *bufio.Reader
	out      io.Writer
	run      func(stdin string, args ...string) error // runs "mpn args..."; stdin is fed to it when not empty
	password func(prompt string) (string, error)      // reads a line without echo
}

type rescueItem struct {
	group string
	label string
	do    func(r *rescueIO) error
}

func rescueItems() []rescueItem {
	return []rescueItem{
		{"Panel access", "Reset admin password", rescueResetPassword},
		{"Panel access", "Turn off admin 2FA", confirmRun("Turn off the two-step sign-in of the administrator?", "web", "2fa-off")},
		{"Panel access", "Disable a delegated user", rescueUserOff},
		{"Panel access", "New first-time setup link", confirmRun("Make a new first-time setup link?", "web", "setup-link")},
		{"Panel access", "Turn off panel SSL (back to the self-signed certificate)", confirmRun("Turn off the panel's certificate and go back to the self-signed one?", "panel", "ssl-off", "-apply")},
		{"Locked out", "Show ports (SSH, panel, phpMyAdmin, webmail)", justRun("port", "status")},
		{"Locked out", "Cancel an unfinished port change", rescuePortCancel},
		{"Locked out", "Unban an IP address", rescueUnban},
		{"Locked out", "Firewall status / turn firewall off", rescueFirewall},
		{"Services", "Status of panel services", justRun("services", "status")},
		{"Services", "Restart the panel (mpn-root, mpn-web, nginx)", rescueRestart},
	}
}

// runRescue is the menu on the real terminal.
func runRescue() {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "The rescue menu needs root. Run: sudo mpn")
		os.Exit(2)
	}
	self, err := os.Executable()
	fail(err)
	r := &rescueIO{
		in:  bufio.NewReader(os.Stdin),
		out: os.Stdout,
		run: func(stdin string, args ...string) error {
			c := exec.Command(self, args...)
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			if stdin != "" {
				c.Stdin = strings.NewReader(stdin)
			} else {
				c.Stdin = os.Stdin
			}
			return c.Run()
		},
		password: func(prompt string) (string, error) {
			fmt.Print(prompt)
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			return string(b), err
		},
	}
	host, _ := os.Hostname()
	rescueLoop(r, fmt.Sprintf("MPN %s rescue menu · host %s", version, host))
}

// isTerminal says whether mpn was started by a person at a terminal, which is when the menu is shown.
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// rescueLoop shows the menu until the person chooses 0 or the input ends.
func rescueLoop(r *rescueIO, title string) {
	items := rescueItems()
	for {
		fmt.Fprintf(r.out, "\n%s\n", title)
		group := ""
		for i, it := range items {
			if it.group != group {
				group = it.group
				fmt.Fprintf(r.out, "\n  %s\n", group)
			}
			fmt.Fprintf(r.out, "  %2d) %s\n", i+1, it.label)
		}
		fmt.Fprintf(r.out, "\n   0) Exit\n")
		line, ok := ask(r, "Choice: ")
		if !ok || line == "0" || line == "q" {
			return
		}
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(items) {
			fmt.Fprintln(r.out, "Choose a number from the list.")
			continue
		}
		if err := items[n-1].do(r); err != nil {
			fmt.Fprintf(r.out, "Failed: %v\n", err)
		}
		if _, ok := ask(r, "\nPress Enter to go back to the menu."); !ok {
			return
		}
	}
}

// ask prints a prompt and reads one trimmed line; ok is false when the input has ended.
func ask(r *rescueIO, prompt string) (string, bool) {
	fmt.Fprint(r.out, prompt)
	line, err := r.in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(r.out)
		return "", false
	}
	return strings.TrimSpace(line), true
}

// yes asks a yes-or-no question whose answer is no unless the person types y.
func yes(r *rescueIO, q string) bool {
	a, ok := ask(r, q+" [y/N] ")
	return ok && (strings.EqualFold(a, "y") || strings.EqualFold(a, "yes"))
}

// runShown prints the command and runs it.
func runShown(r *rescueIO, stdin string, args ...string) error {
	fmt.Fprintf(r.out, "\n$ mpn %s\n", strings.Join(args, " "))
	return r.run(stdin, args...)
}

func justRun(args ...string) func(*rescueIO) error {
	return func(r *rescueIO) error { return runShown(r, "", args...) }
}

func confirmRun(q string, args ...string) func(*rescueIO) error {
	return func(r *rescueIO) error {
		if !yes(r, q) {
			fmt.Fprintln(r.out, "Nothing changed.")
			return nil
		}
		return runShown(r, "", args...)
	}
}

// rescueResetPassword sets a password the person types twice, or generates one when they leave it empty.
func rescueResetPassword(r *rescueIO) error {
	fmt.Fprintf(r.out, "Every browser signed in to the panel will be signed out. 2FA stays as it is.\n")
	pw, err := r.password(fmt.Sprintf("New password (%d to 256 characters, empty to generate one): ", webauth.MinPasswordLen))
	if err != nil {
		return err
	}
	if pw == "" {
		if !yes(r, "Generate a new password and show it once?") {
			fmt.Fprintln(r.out, "Nothing changed.")
			return nil
		}
		return runShown(r, "", "web", "passwd")
	}
	if len(pw) < webauth.MinPasswordLen || len(pw) > 256 {
		return fmt.Errorf("the password must have %d to 256 characters", webauth.MinPasswordLen)
	}
	again, err := r.password("Type it again: ")
	if err != nil {
		return err
	}
	if again != pw {
		return errors.New("the two passwords are not the same; nothing changed")
	}
	return runShown(r, pw+"\n", "web", "passwd", "-stdin")
}

func rescueUserOff(r *rescueIO) error {
	if names := delegatedNames(); len(names) > 0 {
		fmt.Fprintf(r.out, "Delegated users: %s\n", strings.Join(names, ", "))
	}
	name, ok := ask(r, "User name to disable (empty to go back): ")
	if !ok || name == "" {
		return nil
	}
	if !yes(r, fmt.Sprintf("Disable %q and sign them out?", name)) {
		fmt.Fprintln(r.out, "Nothing changed.")
		return nil
	}
	return runShown(r, "", "web", "user-off", name)
}

// delegatedNames lists the delegated users for the prompt; any problem with the files only means no list is shown.
func delegatedNames() []string {
	acct, err := webauth.LoadAccount(defaultAccount)
	if err != nil {
		return nil
	}
	users, err := webauth.LoadUsers(filepath.Join(filepath.Dir(defaultAccount), "users.json"), acct.User)
	if err != nil {
		return nil
	}
	var names []string
	for _, u := range users {
		if !u.Disabled {
			names = append(names, u.Name)
		}
	}
	return names
}

func rescuePortCancel(r *rescueIO) error {
	if err := runShown(r, "", "port", "status"); err != nil {
		return err
	}
	svc, ok := ask(r, "Cancel the change of which port? ssh or panel (empty to go back): ")
	if !ok || svc == "" {
		return nil
	}
	if svc != "ssh" && svc != "panel" {
		return fmt.Errorf("%q: type ssh or panel", svc)
	}
	if !yes(r, fmt.Sprintf("Put the old port of %s back?", svc)) {
		fmt.Fprintln(r.out, "Nothing changed.")
		return nil
	}
	return runShown(r, "", "port", "cancel", "-service", svc, "-apply")
}

func rescueUnban(r *rescueIO) error {
	ip, ok := ask(r, "IP address to unban (empty to go back): ")
	if !ok || ip == "" {
		return nil
	}
	addr, err := security.ValidAddr(ip)
	if err != nil {
		return err
	}
	if !yes(r, fmt.Sprintf("Unban %s from %s?", addr, strings.Join(security.Jails, ", "))) {
		fmt.Fprintln(r.out, "Nothing changed.")
		return nil
	}
	var failed []string
	for _, j := range security.Jails {
		// An address is only banned in some jails, and a jail that is not running refuses; neither is a failure of the unban.
		if err := runShown(r, "", "security", "unban", "-jail", j, "-ip", addr, "-apply"); err != nil {
			failed = append(failed, j)
		}
	}
	if len(failed) == len(security.Jails) {
		return errors.New("no jail took the unban; is fail2ban running? See: mpn security status")
	}
	return nil
}

func rescueFirewall(r *rescueIO) error {
	if err := runShown(r, "", "firewall", "status"); err != nil {
		return err
	}
	if !yes(r, "Turn the firewall off? The server will accept connections on every port until it is turned on again.") {
		fmt.Fprintln(r.out, "Nothing changed.")
		return nil
	}
	return runShown(r, "", "firewall", "disable", "-apply")
}

func rescueRestart(r *rescueIO) error {
	if !yes(r, "Restart mpn-root, mpn-web and nginx? Browsers stay signed in.") {
		fmt.Fprintln(r.out, "Nothing changed.")
		return nil
	}
	fmt.Fprintln(r.out, "\n$ systemctl restart mpn-root mpn-web nginx")
	return exec.Command("systemctl", "restart", "mpn-root", "mpn-web", "nginx").Run()
}
