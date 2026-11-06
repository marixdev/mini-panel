package web

import (
	"regexp"
	"strings"

	"mpn/internal/site"
)

// Where the "back" button of a result goes. A result belongs to the page it was started from: a database made on the
// page of one website goes back to that website's databases, not to the home page. The page that starts the action says
// where, in a field called "back". That field comes from the browser, so it is only taken if it is one of the panel's own
// pages; anything else (another site, a script address, a path with a line break) is ignored and the default is used.

var siteTabs = regexp.MustCompile(`^/sites/([^/]+)(?:/(ssh|databases|backups|files|terminal|ssl|vhost))?$`)

// safeBack returns raw if it is the address of one of the panel's pages, otherwise def.
func safeBack(raw, def string) string {
	switch raw {
	case "/", "/databases", "/backups", "/sites/new":
		return raw
	}
	if m := siteTabs.FindStringSubmatch(raw); m != nil && site.ValidName(m[1]) && !strings.ContainsAny(raw, "\r\n\\?#%") {
		return raw
	}
	return def
}

// siteTab is the address of a tab of one website's page.
func siteTab(name, tab string) string {
	if tab == "" {
		return "/sites/" + name
	}
	return "/sites/" + name + "/" + tab
}
