package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"mpn/internal/provision"
	"mpn/internal/site"
	"mpn/internal/vhost"
)

// customVhostMarker starts the message of a refusal, so that the web panel can recognise it in the output of mpn.
const customVhostMarker = "custom-vhost"

// refuseCustomVhost stops an operation that would write the nginx configuration of a site that has one written by hand. mpn
// makes that file from the domain, the SSL files, the port and the web root; a hand-written one does not follow them, so the
// operation would either do nothing or leave the file saying something that is no longer true.
func refuseCustomVhost(s site.Site, what string) {
	if s.CustomVhost != "" {
		fail(fmt.Errorf("%s: site %q uses an nginx configuration written by hand, so %s would not reach it. Go back to the default on the vHost tab first (mpn site vhost -name %s -reset -apply)",
			customVhostMarker, s.Name, what, s.Name))
	}
}

// vhostInfo is the answer of "mpn site vhost" without flags: what the vHost tab shows.
type vhostInfo struct {
	Custom bool   `json:"custom"`
	Text   string `json:"text"` // the configuration in use: the hand-written one, or the one mpn makes
	TLS    bool   `json:"tls"`
}

// siteVhost shows, replaces or resets the nginx configuration of a site.
//
// With -stdin the text on standard input becomes the configuration. It is written, checked with nginx -t and loaded; when
// nginx refuses it the executor puts the old file back, so the running configuration is never broken by a typing mistake. The
// database is changed only after that worked. With -reset the configuration mpn makes is used again.
func siteVhost(args []string) {
	fs := flag.NewFlagSet("site vhost", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	stdin := fs.Bool("stdin", false, "read the new configuration from standard input")
	reset := fs.Bool("reset", false, "go back to the configuration mpn makes")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only check")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)

	if !*stdin && !*reset {
		text, err := vhost.Render(s, vhost.Options{TLS: s.TLS, CertDir: provision.CertDir(s.Name)})
		fail(err)
		printJSON(vhostInfo{Custom: s.CustomVhost != "", Text: text, TLS: s.TLS})
		return
	}
	if *stdin && *reset {
		fail(errors.New("-stdin and -reset cannot be given together"))
	}
	if *stdin {
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, vhost.MaxCustom+1))
		fail(err)
		s.CustomVhost = string(raw)
		fail(vhost.CheckCustom(s.CustomVhost, s.TLS))
	} else {
		s.CustomVhost = ""
	}
	if !*apply {
		fmt.Println("# checked only. Add -apply to write it, test it with nginx -t and load it.")
		return
	}
	requireLinuxRoot()
	plan, err := provision.VhostPlan(s, "")
	fail(err)
	fail(runPlanQuiet(plan))
	fail(st.SetVhost(s.Name, s.CustomVhost))
	if *reset {
		fmt.Printf("site %q uses the nginx configuration made by mpn again\n", s.Name)
	} else {
		fmt.Printf("site %q uses the nginx configuration you wrote\n", s.Name)
	}
}
