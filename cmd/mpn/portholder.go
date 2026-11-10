package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var nginxListenRe = regexp.MustCompile(`(?m)^\s*listen\s+(?:\[::\]:|[0-9.]+:)?(\d+)`)

// mpnPortHolder says which file of nginx's configuration that mpn wrote listens on port, or "". The static list of ports an app may
// not take (site.ValidAppPort) covers the usual ones; this finds the ports of this very machine, where the panel may have been
// installed on a port of its own choosing and the database browser and the web mail follow it.
func mpnPortHolder(port int) string {
	files, _ := filepath.Glob("/etc/nginx/conf.d/*mpn*.conf")
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range nginxListenRe.FindAllStringSubmatch(string(b), -1) {
			if n, _ := strconv.Atoi(m[1]); n == port {
				return filepath.Base(f)
			}
		}
	}
	return ""
}
