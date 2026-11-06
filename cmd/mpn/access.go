package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"mpn/internal/access"
	"mpn/internal/dbadmin"
	"mpn/internal/panelkey"
	"mpn/internal/site"
)

// installPanelKey lets the web panel log in as the site's account, for its terminal and file manager.
func installPanelKey(s site.Site) error {
	line, err := panelkey.AuthorizedLine(panelkey.Default.Public)
	if err != nil {
		return err
	}
	return access.InstallPanelKey(access.ExecRunner{}, s, line)
}

// siteAccess lets the site's own account log in over SSH or SFTP with a password, or takes that away again, or puts
// back the key the web panel uses. The password is read from standard input, never from an argument.
func siteAccess(args []string) {
	fs := flag.NewFlagSet("site access", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	enable := fs.Bool("enable", false, "allow the site's user to log in with a password")
	disable := fs.Bool("disable", false, "forbid logging in again")
	panel := fs.Bool("panelkey", false, "put the web panel's key back into the account (the account must be able to log in)")
	stdin := fs.Bool("stdin", false, "read the password from the first line of standard input instead of generating one")
	apply := fs.Bool("apply", false, "change it on this machine (Linux, root); otherwise only describe it")
	_ = fs.Parse(args)
	chosen := 0
	for _, b := range []bool{*enable, *disable, *panel} {
		if b {
			chosen++
		}
	}
	if chosen != 1 {
		fail(errors.New("give exactly one of -enable, -disable and -panelkey"))
	}
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	if !*apply {
		switch {
		case *enable:
			fmt.Printf("would let user %q (site %q) log in over SSH/SFTP with a password, and let the web panel log in as it\n", s.User, s.Name)
		case *disable:
			fmt.Printf("would stop user %q (site %q) from logging in, lock its password and remove the panel's key\n", s.User, s.Name)
		default:
			fmt.Printf("would put the web panel's key back into the account of user %q (site %q)\n", s.User, s.Name)
		}
		fmt.Println("(dry run: add -apply to do it)")
		return
	}
	requireLinuxRoot()
	if *disable {
		fail(access.Disable(access.ExecRunner{}, s))
		fmt.Printf("user %q can no longer log in\n", s.User)
		return
	}
	if *panel {
		if !access.Enabled("/etc/passwd", s.User) {
			fail(fmt.Errorf("user %q cannot log in; enable the login first", s.User))
		}
		fail(installPanelKey(s))
		fmt.Printf("the web panel can log in as %q again\n", s.User)
		return
	}
	var pw string
	if *stdin {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		pw = strings.TrimRight(line, "\r\n")
		if pw == "" {
			fail(errors.New("no password on standard input"))
		}
	} else {
		pw, err = dbadmin.NewPassword()
		fail(err)
	}
	fail(access.Enable(access.ExecRunner{}, s, pw))
	if err := installPanelKey(s); err != nil {
		// The login itself works; only the panel's terminal and file manager need the key.
		fmt.Fprintf(os.Stderr, "warning: the web panel's key was not installed: %v\n", err)
	}
	if *stdin {
		fmt.Printf("user %q can now log in with the password you gave\n", s.User)
		return
	}
	fmt.Printf("user %q can now log in over SSH/SFTP\n\n  user:     %s\n  password: %s\n\nSave it now: mpn does not keep it.\n", s.User, s.User, pw)
}
