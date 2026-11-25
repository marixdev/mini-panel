package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/dbadmin"
	"mpn/internal/installer"
	"mpn/internal/monitor"
	"mpn/internal/notify"
	"mpn/internal/panelkey"
	"mpn/internal/panelssh"
	"mpn/internal/panelssl"
	"mpn/internal/site"
	"mpn/internal/svcport"
	"mpn/internal/unit"
	"mpn/internal/web"
	"mpn/internal/webauth"
)

const defaultAccount = "/var/lib/mpn-web/account.json"

var webUserRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// runWeb starts the web panel, or with "passwd" sets the administrator's password.
func runWeb(args []string) {
	if len(args) > 0 && args[0] == "login-test" {
		webLoginTest(args[1:])
		return
	}
	if len(args) > 0 && args[0] == "passwd" {
		webPasswd(args[1:])
		return
	}
	if len(args) > 0 && args[0] == "2fa-off" {
		webTwoFAOff(args[1:])
		return
	}
	if len(args) > 0 && args[0] == "setup-link" {
		webSetupLink(args[1:])
		return
	}
	webServe(args)
}

func webServe(args []string) {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	listen := fs.String("listen", "systemd", `where to listen: "systemd" (the socket of mpn-web.socket), "unix:PATH", or a loopback address for development`)
	account := fs.String("account", defaultAccount, "file with the administrator account")
	var hosts hostList
	fs.Var(&hosts, "host", "name the panel is reached by, repeat for several (localhost is always accepted)")
	secure := fs.Bool("secure", false, "the panel is reached over HTTPS: Secure cookie and HSTS")
	trustProxy := fs.Bool("trust-proxy", false, "take the client address from X-Real-IP, which nginx on this machine sets")
	socket := fs.String("socket", "/run/mpn/root.sock", "unix socket of the root helper (mpn root)")
	sshAddr := fs.String("ssh", "auto", `SSH server of this machine, used to log in as a site's account with the panel's key; "auto" asks the root helper for the port sshd listens on`)
	pmaPort := fs.Int("pma-port", 0, "port of the database browser (phpMyAdmin) on this machine; 0 when it is not installed")
	pmaKeyFile := fs.String("pma-key", "/etc/mpn/pma/secret", "file with the key that the sign-on page of the database browser proves itself with")
	_ = fs.Parse(args)
	var pmaKey []byte
	if *pmaPort != 0 {
		if *pmaPort < 1024 || *pmaPort > 65535 {
			fail(fmt.Errorf("-pma-port %d must be between 1024 and 65535", *pmaPort))
		}
		b, err := os.ReadFile(*pmaKeyFile)
		fail(err)
		pmaKey = []byte(strings.TrimSpace(string(b)))
		if len(pmaKey) < 32 {
			fail(fmt.Errorf("%s: the key must be at least 32 characters", *pmaKeyFile))
		}
	}

	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		fail(errors.New("the web panel must not run as root; run it as the mpn-web user"))
	}
	ln, trusted, err := webListener(*listen)
	fail(err)
	if *trustProxy && !trusted {
		// On a loopback port any program of the machine can connect and say any client address, which would let it get around the
		// limit on sign-in attempts or have fail2ban ban the administrator.
		fmt.Fprintln(os.Stderr, "-trust-proxy is ignored on a TCP port: only a socket that only nginx can open makes X-Real-IP believable")
		*trustProxy = false
	}
	// When the account file is missing the panel can still start: it will show a first-time setup
	// page and write the file itself, since it runs as the account that owns the directory.
	// An account file that cannot be read is said in the log, and the panel still starts: every sign-in fails and the setup page
	// stays closed until root repairs it. Stopping instead would leave nginx with a panel that restarts for ever (504).
	accountExists := true
	if _, err := webauth.LoadAccount(*account); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("the account cannot be read, nobody can sign in until it is repaired (as root: mpn web passwd): %v", err)
		}
		accountExists = false
	}
	_ = accountExists // passed to the server below
	// The measurements are kept next to the account file: the one place the panel is allowed to write.
	web.Version = version
	// The terminal and the file manager reach SSH on the port sshd listens on, which the provider may have set to something other
	// than 22. The panel cannot run sshd -T itself, so it asks the root helper, and asks again when the port may have changed.
	sshDialer := &panelssh.Auto{
		Client: panelssh.Client{KeyPath: panelkey.Default.Private, HostKeyGlob: "/etc/ssh/ssh_host_*_key.pub", Timeout: 8 * time.Second},
		Fixed:  *sshAddr,
		Lookup: func() (string, error) {
			resp, err := ctl.Call(*socket, ctl.Request{Op: "port.sshaddr"}, 15*time.Second)
			if err != nil {
				return "", err
			}
			if !resp.OK {
				return "", errors.New(resp.Error)
			}
			return strings.TrimSpace(resp.Output), nil
		},
	}
	collector := monitor.New(monitor.ProcSource, filepath.Join(filepath.Dir(*account), "metrics.json"))
	// A sign-in lasts 2 hours without use and 24 hours in all; a browser the person asked to remember stays signed in for 30 days.
	// Both are kept next to the account, so an update or a restart of the panel signs nobody out.
	sessions := webauth.NewSessions(2*time.Hour, 24*time.Hour, 20)
	sessions.LongIdle, sessions.LongMax = 30*24*time.Hour, 30*24*time.Hour
	sessions.File = filepath.Join(filepath.Dir(*account), "sessions.json")
	sessions.Load()
	// The panel looks for a new release by itself and keeps what it found next to the account, so a restart does not ask again at once.
	updates := &web.UpdateChecker{File: filepath.Join(filepath.Dir(*account), "update-check.json")}
	updates.Load()
	srv := &web.Server{
		Mon:     collector,
		Account: func() (webauth.Account, error) { return webauth.LoadAccount(*account) },
		// The account file is in the one folder the panel may write to and belongs to the panel's own account, so a new password or
		// the secret of the two-step sign-in is saved by the panel itself.
		SaveAccount:   func(a webauth.Account) error { return webauth.SaveAccount(*account, a, -1, -1) },
		CreateAccount: func(a webauth.Account) error { return webauth.CreateAccount(*account, a) },
		SetupKeyFile:  setupKeyPath(*account),
		// The file of the panel's own name on port 443; when it is there, the database browser is opened under /pma/ of that name.
		PanelDomainFile: installer.PanelDomainFile,
		NotifySocket:    notify.Socket,
		Updates:         updates,
		Sessions:        sessions,
		Throttle:        webauth.NewThrottle(),
		Hosts:           hosts,
		HostsFile:       panelssl.HostsFile,
		PMAPort:         *pmaPort,
		PMAKey:          pmaKey,
		Secure:          *secure,
		TrustProxy:      *trustProxy,
		Dialer:          sshDialer,
		InstalledPHP:    site.InstalledPHP,
		NodeRoot:        unit.NodeRoot,
		TermLog: func(event, site string, err error) {
			if err != nil {
				log.Printf("terminal %s: site %s: %v", event, site, err)
				return
			}
			log.Printf("terminal %s: site %s", event, site)
		},
		LoginCheck: func(user string) error {
			c, err := sshDialer.DialTimeout(user, 4*time.Second)
			if err != nil {
				return err
			}
			return c.Close()
		},
		Call: func(r ctl.Request) (ctl.Response, error) {
			return ctl.Call(*socket, r, 25*time.Minute)
		},
	}
	hs := &http.Server{
		Handler: srv.Handler(), MaxHeaderBytes: 16 << 10,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 25 * time.Minute, // an operation such as a restore can take a while
		IdleTimeout:  60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go collector.Run(ctx)
	go updates.Run(ctx)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	fmt.Fprintf(os.Stderr, "panel listening on %s %s\n", ln.Addr().Network(), ln.Addr())
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fail(err)
	}
}

// webPasswd sets the administrator's password. It runs as root and gives the account file to the web user.
func webPasswd(args []string) {
	fs := flag.NewFlagSet("web passwd", flag.ExitOnError)
	account := fs.String("account", defaultAccount, "file with the administrator account")
	name := fs.String("user", "admin", "administrator's user name")
	webUser := fs.String("web-user", "mpn-web", "account the panel runs as; it will own the file")
	stdin := fs.Bool("stdin", false, "read the password from the first line of standard input instead of generating one")
	_ = fs.Parse(args)
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fail(errors.New("setting the password needs Linux and root"))
	}
	if !webUserRe.MatchString(*name) {
		fail(fmt.Errorf("invalid user name %q", *name))
	}
	u, err := user.Lookup(*webUser)
	fail(err)
	uid, err := strconv.Atoi(u.Uid)
	fail(err)
	gid, err := strconv.Atoi(u.Gid)
	fail(err)
	if uid == 0 {
		fail(errors.New("the web account must not be root"))
	}
	var pw string
	if *stdin {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		pw = strings.TrimRight(line, "\r\n")
		if len(pw) < webauth.MinPasswordLen || len(pw) > 256 {
			fail(fmt.Errorf("the password must have %d to 256 characters", webauth.MinPasswordLen))
		}
	} else {
		pw, err = dbadmin.NewPassword()
		fail(err)
	}
	hash, err := webauth.Hash(pw)
	fail(err)
	dir := strings.TrimSuffix(*account, "/"+lastElem(*account))
	fail(os.MkdirAll(dir, 0o750))
	fail(os.Chown(dir, uid, gid))
	fail(os.Chmod(dir, 0o750))
	acct := webauth.Account{User: *name, Hash: hash}
	if old, err := webauth.LoadAccount(*account); err == nil && old.User == *name {
		acct.TOTP, acct.TOTPStep = old.TOTP, old.TOTPStep // a new password does not switch the second step off: "mpn web 2fa-off" does
	}
	fail(webauth.SaveAccount(*account, acct, uid, gid))
	// A new password ends every sign-in, the remembered ones too. The sessions are kept in a file so they outlive a restart of the
	// panel; removing the file and restarting the panel is what ends them.
	sessFile := filepath.Join(filepath.Dir(*account), "sessions.json")
	if err := os.Remove(sessFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		fail(err)
	}
	restarted := exec.Command("systemctl", "try-restart", "mpn-web.service").Run() == nil
	if *stdin {
		fmt.Printf("password of %q set\n", *name)
	} else {
		fmt.Printf("panel login\n\n  user:     %s\n  password: %s\n\nSave it now: only a hash is stored. Set a new one with:  mpn web passwd\n", *name, pw)
	}
	if restarted {
		fmt.Println("Every browser that was signed in has been signed out.")
	} else {
		fmt.Println("Restart the panel to sign out every browser: systemctl restart mpn-web")
	}
}

// webTwoFAOff switches the two-step sign-in off, for somebody who has lost the app. It runs as root on the machine, which is
// what makes it safe to offer: a person who can run it already controls the panel. The panel reads the account on each sign-in,
// so it takes effect at once.
func webTwoFAOff(args []string) {
	fs := flag.NewFlagSet("web 2fa-off", flag.ExitOnError)
	account := fs.String("account", defaultAccount, "file with the administrator account")
	webUser := fs.String("web-user", "mpn-web", "account the panel runs as; it will own the file")
	_ = fs.Parse(args)
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fail(errors.New("switching the two-step sign-in off needs Linux and root"))
	}
	acct, err := webauth.LoadAccount(*account)
	fail(err)
	if acct.TOTP == "" {
		fmt.Println("the two-step sign-in is already off")
		return
	}
	u, err := user.Lookup(*webUser)
	fail(err)
	uid, err := strconv.Atoi(u.Uid)
	fail(err)
	gid, err := strconv.Atoi(u.Gid)
	fail(err)
	if uid == 0 {
		fail(errors.New("the web account must not be root"))
	}
	acct.TOTP, acct.TOTPStep = "", 0
	fail(webauth.SaveAccount(*account, acct, uid, gid))
	fmt.Printf("the two-step sign-in of %q is off; sign in with the password, then turn it on again in Settings if you want it\n", acct.User)
}

func lastElem(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// webLoginTest logs in as a site's account the way the panel does, with the panel's key, and prints who it is. It is for
// checking a server by hand; run it as the web account, because only that account can read the key.
func webLoginTest(args []string) {
	fs := flag.NewFlagSet("web login-test", flag.ExitOnError)
	userName := fs.String("user", "", "account to log in as")
	addr := fs.String("ssh", "", "SSH server of this machine; empty finds the port sshd listens on")
	_ = fs.Parse(args)
	if *addr == "" {
		*addr = svcport.DialAddr(sshListenPorts(), svcport.Listeners(out(5*time.Second, "ss", "-ltnH")))
	}
	c, err := panelssh.Client{Addr: *addr, KeyPath: panelkey.Default.Private, HostKeyGlob: "/etc/ssh/ssh_host_*_key.pub"}.Dial(*userName)
	fail(err)
	defer c.Close()
	sess, err := c.NewSession()
	fail(err)
	defer sess.Close()
	out, err := sess.Output("id -un")
	fail(err)
	fmt.Print(string(out))
}
