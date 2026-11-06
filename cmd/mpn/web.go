package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
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
	"mpn/internal/monitor"
	"mpn/internal/panelkey"
	"mpn/internal/panelssh"
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
	webServe(args)
}

func webServe(args []string) {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:8780", "address to listen on; only loopback addresses are accepted, put nginx with TLS in front")
	account := fs.String("account", defaultAccount, "file with the administrator account")
	var hosts hostList
	fs.Var(&hosts, "host", "name the panel is reached by, repeat for several (localhost is always accepted)")
	secure := fs.Bool("secure", false, "the panel is reached over HTTPS: Secure cookie and HSTS")
	trustProxy := fs.Bool("trust-proxy", false, "take the client address from X-Real-IP, which nginx on this machine sets")
	socket := fs.String("socket", "/run/mpn/root.sock", "unix socket of the root helper (mpn root)")
	sshAddr := fs.String("ssh", "127.0.0.1:22", "SSH server of this machine, used to log in as a site's account with the panel's key")
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

	host, _, err := net.SplitHostPort(*listen)
	fail(err)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		fail(fmt.Errorf("-listen %s: the panel only listens on a loopback address; reach it through nginx with TLS", *listen))
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		fail(errors.New("the web panel must not run as root; run it as the mpn-web user"))
	}
	if _, err := webauth.LoadAccount(*account); err != nil {
		fail(fmt.Errorf("%w\ncreate the account as root with: mpn web passwd", err))
	}
	// The measurements are kept next to the account file: the one place the panel is allowed to write.
	collector := monitor.New(monitor.ProcSource, filepath.Join(filepath.Dir(*account), "metrics.json"))
	srv := &web.Server{
		Mon:        collector,
		Account:    func() (webauth.Account, error) { return webauth.LoadAccount(*account) },
		Sessions:   webauth.NewSessions(30*time.Minute, 12*time.Hour, 20),
		Throttle:   webauth.NewThrottle(),
		Hosts:      hosts,
		PMAPort:    *pmaPort,
		PMAKey:     pmaKey,
		Secure:     *secure,
		TrustProxy: *trustProxy,
		Dialer:     panelssh.Client{Addr: *sshAddr, KeyPath: panelkey.Default.Private, HostKeyGlob: "/etc/ssh/ssh_host_*_key.pub", Timeout: 8 * time.Second},
		PHPDir:     "/usr/sbin",
		NodeRoot:   unit.NodeRoot,
		TermLog: func(event, site string, err error) {
			if err != nil {
				log.Printf("terminal %s: site %s: %v", event, site, err)
				return
			}
			log.Printf("terminal %s: site %s", event, site)
		},
		LoginCheck: func(user string) error {
			c, err := panelssh.Client{Addr: *sshAddr, KeyPath: panelkey.Default.Private, HostKeyGlob: "/etc/ssh/ssh_host_*_key.pub", Timeout: 4 * time.Second}.Dial(user)
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
		Addr: *listen, Handler: srv.Handler(), MaxHeaderBytes: 16 << 10,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 25 * time.Minute, // an operation such as a restore can take a while
		IdleTimeout:  60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go collector.Run(ctx)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	fmt.Fprintf(os.Stderr, "panel listening on http://%s\n", *listen)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
	fail(webauth.SaveAccount(*account, webauth.Account{User: *name, Hash: hash}, uid, gid))
	if *stdin {
		fmt.Printf("password of %q set\n", *name)
	} else {
		fmt.Printf("panel login\n\n  user:     %s\n  password: %s\n\nSave it now: only a hash is stored. Set a new one with:  mpn web passwd\n", *name, pw)
	}
	fmt.Println("A running panel keeps its signed-in sessions until they expire; restart it to end them: systemctl restart mpn-web")
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
	addr := fs.String("ssh", "127.0.0.1:22", "SSH server of this machine")
	_ = fs.Parse(args)
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
