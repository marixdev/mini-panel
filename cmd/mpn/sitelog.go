package main

import (
	"flag"
	"fmt"
	"time"

	"mpn/internal/provision"
	"mpn/internal/site"
	"mpn/internal/sitelog"
	"mpn/internal/unit"
)

// siteLogCmd prints the end of one log of a website as JSON, for the Logs tab. It only reads. The nginx logs are the files in the
// site's log folder; the log of a Node or Python site's own program is its service's journal.
func siteLogCmd(args []string) {
	fs := flag.NewFlagSet("site log", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	kind := fs.String("kind", sitelog.Access, "access, error or app")
	lines := fs.Int("lines", sitelog.DefaultLines, "how many of the last lines to print")
	query := fs.String("query", "", "print only the lines that contain this text")
	_ = fs.Parse(args)

	fail(sitelog.ValidKind(*kind))
	n, err := sitelog.ValidLines(*lines)
	fail(err)
	fail(sitelog.ValidQuery(*query))
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)

	res := sitelog.Result{Kind: *kind, Lines: []string{}}
	var text string
	switch *kind {
	case sitelog.App:
		if s.Kind != site.Node && s.Kind != site.Python {
			fail(fmt.Errorf("site %q is %s: only a node or python site has a program of its own", s.Name, s.Kind))
		}
		res.Path = unit.Name(s)
		res.Exists = true
		// The unit name is made from a validated site name; every other argument is fixed.
		text = out(10*time.Second, "journalctl", "-u", res.Path, "-n", fmt.Sprint(sitelog.MaxLines), "--no-pager", "-o", "short-iso", "-q")
	default:
		res.Path = provision.LogDir(s.Name) + "/" + *kind + ".log"
		data, size, exists, err := sitelog.Tail(res.Path)
		fail(err)
		res.Exists, res.Size = exists, size
		res.Truncated = exists && int64(len(data)) < size
		text = string(data)
	}
	res.Lines, res.Matched = sitelog.Select(text, n, *query)
	printJSON(res)
}
