package ctl

import (
	"fmt"
	"strings"

	"mpn/internal/apps"
)

// wpArgv checks the request of the WordPress manager with the same rules the program applies again, and attaches each value to its flag
// with "=".
func (r Request) wpArgv() ([]string, error) {
	if err := requireSite(r.Site); err != nil {
		return nil, err
	}
	switch r.Op {
	case "wp.status":
		return []string{"wp", "status", "-json", "-site=" + r.Site}, nil
	case "wp.search":
		if _, err := apps.SearchArgs(r.WPKind, r.WPTerm); err != nil {
			return nil, err
		}
		return []string{"wp", "search", "-site=" + r.Site, "-kind=" + r.WPKind, "-term=" + strings.TrimSpace(r.WPTerm)}, nil
	}
	if _, err := apps.ManageArgs(r.WPKind, r.WPAction, r.WPSlug); err != nil {
		return nil, err
	}
	return []string{"wp", "run", "-site=" + r.Site, "-kind=" + r.WPKind, "-action=" + r.WPAction, "-slug=" + r.WPSlug, "-apply"}, nil
}

// installApp checks the application and, for WordPress, the details of the administrator, and turns them into the arguments of
// "mpn app install". Every value is attached to its flag with "=", so one that starts with "-" cannot be read as a flag.
func (r Request) installApp() ([]string, error) {
	if err := requireSite(r.Site); err != nil {
		return nil, err
	}
	rec, ok := apps.Get(r.App)
	if !ok {
		return nil, fmt.Errorf("application %q is not one of %v", r.App, apps.IDs())
	}
	a := []string{"app", "install", "-site=" + r.Site, "-app=" + rec.ID}
	if rec.ID == apps.WordPressID {
		switch {
		case !apps.ValidTitle(r.AppTitle):
			return nil, fmt.Errorf("the site title must be one line of at most 100 characters")
		case !apps.ValidAdminUser(r.AppAdmin):
			return nil, fmt.Errorf("the administrator name must have 3 to 60 letters, digits or . _ @ -")
		case !apps.ValidEmail(r.AppEmail):
			return nil, fmt.Errorf("the administrator email is not valid")
		case !apps.ValidLocale(r.AppLocale):
			return nil, fmt.Errorf("the language %q is not offered", r.AppLocale)
		}
		a = append(a, "-title="+r.AppTitle, "-admin="+r.AppAdmin, "-email="+r.AppEmail, "-locale="+r.AppLocale)
	}
	return append(a, "-apply"), nil
}
