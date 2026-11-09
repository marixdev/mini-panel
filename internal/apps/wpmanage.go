package apps

import (
	"fmt"
	"regexp"
	"strings"
)

// The slugs of wordpress.org are lower-case letters, digits and hyphens. A slug to install is held to that; one to act on may also have the
// dots and underscores that an installed plugin file can carry.
var (
	installSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	itemSlugRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	searchTermRe  = regexp.MustCompile(`^[\p{L}\p{N} ._+-]{1,60}$`)
)

func ValidInstallSlug(s string) bool { return installSlugRe.MatchString(s) }
func ValidItemSlug(s string) bool    { return itemSlugRe.MatchString(s) && !strings.Contains(s, "..") }
func ValidSearchTerm(s string) bool {
	return searchTermRe.MatchString(strings.TrimSpace(s)) && strings.TrimSpace(s) != ""
}

// WP-CLI is asked for these fields only, so the answer is short and does not change with its version.
const (
	PluginFields = "name,title,status,version,update,update_version"
	ThemeFields  = "name,title,status,version,update,update_version"
)

// ManageArgs turns one action of the manager into the arguments of WP-CLI, or says why it is refused. kind is plugin, theme or core;
// action is what to do with it.
func ManageArgs(kind, action, slug string) ([]string, error) {
	switch kind {
	case "core":
		if action != "update" || slug != "" {
			return nil, fmt.Errorf("WordPress itself can only be updated")
		}
		return []string{"core", "update"}, nil
	case "plugin", "theme":
	default:
		return nil, fmt.Errorf("%q is not plugin, theme or core", kind)
	}
	switch action {
	case "install":
		if !ValidInstallSlug(slug) {
			return nil, fmt.Errorf("%q is not a plugin or theme name from wordpress.org", slug)
		}
		return []string{kind, "install", slug}, nil
	case "activate", "deactivate", "update", "delete":
		if !ValidItemSlug(slug) {
			return nil, fmt.Errorf("%q is not the name of an installed %s", slug, kind)
		}
		if kind == "theme" && action == "deactivate" {
			return nil, fmt.Errorf("a theme cannot be deactivated; activate another one")
		}
		return []string{kind, action, slug}, nil
	}
	return nil, fmt.Errorf("%q is not an action of the manager", action)
}

// SearchArgs is the search of wordpress.org for plugins or themes.
func SearchArgs(kind, term string) ([]string, error) {
	if kind != "plugin" && kind != "theme" {
		return nil, fmt.Errorf("%q is not plugin or theme", kind)
	}
	term = strings.TrimSpace(term)
	if !ValidSearchTerm(term) {
		return nil, fmt.Errorf("the search words may have letters, digits, spaces and . _ + - only, up to 60 characters")
	}
	fields := "name,slug,rating,version,short_description"
	if kind == "theme" {
		fields = "name,slug,rating,version"
	}
	return []string{kind, "search", term, "--per-page=12", "--format=json", "--fields=" + fields}, nil
}
