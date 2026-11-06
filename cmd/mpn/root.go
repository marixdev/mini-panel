package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/panelkey"
)

// runRoot is the privileged half of the web panel. It listens on a unix socket that only the web user can open, and it
// runs a request only if ctl.Request.Argv accepts it, by starting this same binary with the checked arguments. It does
// not serve HTTP and reads nothing from the network.
func runRoot(args []string) {
	fs := flag.NewFlagSet("root", flag.ExitOnError)
	sock := fs.String("socket", "/run/mpn/root.sock", "unix socket to listen on")
	webUser := fs.String("web-user", "mpn-web", "the only account allowed to send requests; its group owns the socket")
	_ = fs.Parse(args)
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		fail(errors.New("the root helper needs Linux and root"))
	}
	u, err := user.Lookup(*webUser)
	fail(err)
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	fail(err)
	if uid == 0 {
		fail(errors.New("the web account must not be root"))
	}
	g, err := user.LookupGroupId(u.Gid)
	fail(err)
	gid, err := strconv.Atoi(u.Gid)
	fail(err)
	created, err := panelkey.Ensure(panelkey.Default, int(uid), gid)
	fail(err)
	if created {
		log.Printf("made the SSH key pair of the web panel; sites with a login need it installed again (mpn site access -panelkey)")
	}
	l, err := ctl.Listen(*sock, g.Name)
	fail(err)
	srv := &ctl.Server{
		Runner:  ctl.ExecRunner{Binary: "/usr/local/bin/mpn", Max: 256 << 10},
		Allow:   ctl.AllowUIDs(uint32(uid)),
		Peer:    ctl.PeerUID,
		Timeout: 20 * time.Minute,
		Log: func(argv []string, err error) {
			if err != nil {
				log.Printf("failed: mpn %s: %v", strings.Join(argv, " "), err)
				return
			}
			log.Printf("ok: mpn %s", strings.Join(argv, " "))
		},
	}
	log.Printf("listening on %s for user %s (uid %d)", *sock, *webUser, uid)
	fail(srv.Serve(l))
}
