package web

import (
	"net"
	"os/user"
	"strings"

	"mpn/internal/site"
)

// secondLevel are the labels that sit in front of a country code in names such as example.co.uk.
var secondLevel = map[string]bool{"co": true, "com": true, "net": true, "org": true, "gov": true, "edu": true, "ac": true}

// siteUserFrom makes the name of a site's system account from its first host name: the top-level domain goes, and so
// does a leading "www.". "datvu.io" gives "datvu", "www.example.co.uk" gives "example", "blog.example.com" gives
// "blog-example". The rule is the same one app.js uses while the user types, so the form shows what the server would pick.
func siteUserFrom(host string) string {
	h := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), "www.")
	labels := strings.Split(h, ".")
	if len(labels) >= 2 {
		tld := labels[len(labels)-1]
		labels = labels[:len(labels)-1]
		if len(tld) == 2 && len(labels) >= 2 && secondLevel[labels[len(labels)-1]] {
			labels = labels[:len(labels)-1]
		}
	}
	var b strings.Builder
	for _, c := range strings.Join(labels, "-") {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	u := strings.Trim(b.String(), "-")
	if u == "" {
		return "site"
	}
	if u[0] >= '0' && u[0] <= '9' {
		u = "u" + u // an account name may not start with a digit
	}
	if len(u) > 32 {
		u = strings.TrimRight(u[:32], "-")
	}
	if site.IsReservedUser(u) {
		u = strings.TrimRight(u[:min(len(u), 28)], "-") + "-web"
	}
	return u
}

// accountExists reports whether the machine already has an account with that name.
func (s *Server) accountExists(name string) bool {
	if s.UserExists != nil {
		return s.UserExists(name)
	}
	_, err := user.Lookup(name)
	return err == nil
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

func reservedList() string { return strings.Join(site.ReservedUsers(), ",") }
