package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"mpn/internal/apps"
	"mpn/internal/site"
)

func wpUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn wp <status|run|search> -site NAME [flags]   (add -h for the flags)")
	os.Exit(2)
}

// wpSite returns the site, which must be a WordPress site installed by mpn.
func wpSite(name string) site.Site {
	st := openStore(defaultDB)
	defer st.Close()
	s, err := st.GetSite(name)
	fail(err)
	if s.App != apps.WordPressID {
		fail(errors.New("wp-notwp: this website has no WordPress installed by mpn"))
	}
	return s
}

// wpRun runs WP-CLI in the WordPress of a site, as the site's own account, and returns what it printed. The arguments are a list for
// exec, never a shell line.
func wpRun(s site.Site, timeout time.Duration, args ...string) (stdout, stderr string, err error) {
	argv := apps.WPArgv(s, args...)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	cmd.WaitDelay = 5 * time.Second
	err = cmd.Run()
	return strings.TrimSpace(o.String()), strings.TrimSpace(e.String()), err
}

type wpItem struct {
	Name          string `json:"name"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	Update        string `json:"update"`
	UpdateVersion string `json:"update_version"`
}

// wpStatus is the answer of "mpn wp status -json".
type wpStatus struct {
	Version    string   `json:"version"`
	URL        string   `json:"url"`
	CoreUpdate string   `json:"core_update"` // the version a core update would bring, or ""
	Plugins    []wpItem `json:"plugins"`
	Themes     []wpItem `json:"themes"`
}

func wpList(s site.Site, kind, fields string) ([]wpItem, error) {
	out, errOut, err := wpRun(s, 60*time.Second, kind, "list", "--format=json", "--fields="+fields)
	if err != nil {
		return nil, fmt.Errorf("wp %s list: %s", kind, firstLine(errOut, err))
	}
	items := []wpItem{}
	if out != "" {
		if err := json.Unmarshal([]byte(out), &items); err != nil {
			return nil, fmt.Errorf("wp %s list gave an answer that is not JSON", kind)
		}
	}
	return items, nil
}

func firstLine(text string, err error) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	if text == "" {
		return err.Error()
	}
	return text
}

func runWP(args []string) {
	if len(args) == 0 {
		wpUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("wp "+sub, flag.ExitOnError)
	name := fs.String("site", "", "the website")
	kind := fs.String("kind", "", "run, search: plugin, theme or core")
	action := fs.String("action", "", "run: install, activate, deactivate, update or delete (core: update)")
	slug := fs.String("slug", "", "run: the plugin or theme")
	term := fs.String("term", "", "search: the words to look for")
	asJSON := fs.Bool("json", false, "status: print JSON")
	apply := fs.Bool("apply", false, "run: do it (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])
	if !site.ValidName(*name) {
		fail(fmt.Errorf("invalid site name %q", *name))
	}

	switch sub {
	case "status":
		s := wpSite(*name)
		var st wpStatus
		var err error
		st.Version, _, err = wpRun(s, 60*time.Second, "core", "version")
		if err != nil {
			fail(errors.New("wp-broken: WP-CLI cannot read this WordPress; its files may be damaged or moved"))
		}
		st.URL, _, _ = wpRun(s, 30*time.Second, "option", "get", "siteurl")
		fields := "name,title,status,version,update,update_version"
		if st.Plugins, err = wpList(s, "plugin", fields); err != nil {
			fail(err)
		}
		if st.Themes, err = wpList(s, "theme", fields); err != nil {
			fail(err)
		}
		// A core update is looked for on wordpress.org; when that cannot be reached the page simply shows none.
		if out, _, err := wpRun(s, 40*time.Second, "core", "check-update", "--format=json"); err == nil && strings.HasPrefix(out, "[") {
			var up []struct {
				Version string `json:"version"`
			}
			if json.Unmarshal([]byte(out), &up) == nil && len(up) > 0 {
				st.CoreUpdate = up[0].Version
			}
		}
		if *asJSON {
			printJSON(st)
			return
		}
		fmt.Printf("WordPress %s at %s, %d plugins, %d themes\n", st.Version, st.URL, len(st.Plugins), len(st.Themes))
	case "run":
		argv, err := apps.ManageArgs(*kind, *action, *slug)
		fail(err)
		if !*apply {
			fmt.Printf("would run: wp %s\n", strings.Join(argv, " "))
			return
		}
		requireLinuxRoot()
		s := wpSite(*name)
		out, errOut, err := wpRun(s, 10*time.Minute, argv...)
		if out != "" {
			fmt.Println(out)
		}
		if err != nil {
			msg := errOut
			if msg == "" {
				msg = err.Error()
			}
			fail(errors.New(msg))
		}
	case "search":
		argv, err := apps.SearchArgs(*kind, *term)
		fail(err)
		s := wpSite(*name)
		out, errOut, err := wpRun(s, 60*time.Second, argv...)
		if err != nil {
			msg := errOut
			if msg == "" {
				msg = err.Error()
			}
			fail(errors.New("wp-search: " + firstLine(msg, err)))
		}
		if out == "" || !strings.HasPrefix(out, "[") {
			out = "[]"
		}
		fmt.Println(out)
	default:
		wpUsage()
	}
}
