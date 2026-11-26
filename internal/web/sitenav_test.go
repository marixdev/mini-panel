package web

import (
	"strings"
	"testing"

	"mpn/internal/webauth"
)

func menuKeys(menu []siteTabGroup) (groups []string, tabs []string) {
	for _, g := range menu {
		groups = append(groups, g.Key)
		for _, t := range g.Tabs {
			tabs = append(tabs, t.Key)
		}
	}
	return groups, tabs
}

func TestSiteMenuGroupsEveryTabOnce(t *testing.T) {
	groups, tabs := menuKeys(siteMenu(siteRow{Kind: "php"}))
	if strings.Join(groups, ",") != "site,data,run,advanced" {
		t.Errorf("groups %v", groups)
	}
	want := "overview,files,email,ssl,databases,backups,cron,background,logs,activity,terminal,vhost"
	if strings.Join(tabs, ",") != want {
		t.Errorf("tabs %v, want %s", tabs, want)
	}
}

func TestSiteMenuLeavesOutWhatTheSiteCannotHave(t *testing.T) {
	_, tabs := menuKeys(siteMenu(siteRow{Kind: "static"}))
	for _, k := range tabs {
		if k == "background" {
			t.Error("a static site has no background services")
		}
	}
	_, tabs = menuKeys(siteMenu(siteRow{Kind: "php", App: "wordpress"}))
	if len(tabs) < 2 || tabs[1] != "wordpress" {
		t.Errorf("WordPress should follow the overview: %v", tabs)
	}
}

func TestSitePageHasTheSideMenuWithGroupsAndTheOpenTab(t *testing.T) {
	h, c := hidingSessions(t)
	page := hidingPage(t, h, c["admin"], "/sites/blog/backups")
	for _, want := range []string{`class="site-layout"`, `class="site-nav"`, `class="site-nav-heading"`,
		`href="/sites/blog/background"`, `href="/sites/blog/terminal"`} {
		if !strings.Contains(page, want) {
			t.Errorf("site page lacks %s", want)
		}
	}
	if strings.Count(page, `href="/sites/blog/backups" aria-current="page"`) != 2 {
		t.Error("the open tab is not marked in both the narrow and the wide menu")
	}
	if !strings.Contains(page, `class="site-nav-wide"`) {
		t.Error("the wide menu, outside the details element, is missing")
	}
	if strings.Contains(page, `class="site-tabs"`) {
		t.Error("the old strip of tabs is still drawn")
	}
}

func TestSideMenuLeavesTheTerminalOutForUsersBelowFull(t *testing.T) {
	h, c := hidingSessions(t)
	page := hidingPage(t, h, c["manage"], "/sites/blog")
	if strings.Contains(page, `href="/sites/blog/terminal"`) {
		t.Error("a manage user sees the terminal")
	}
	if !strings.Contains(page, `href="/sites/blog/vhost"`) {
		t.Error("the Advanced group lost the vHost entry")
	}
}

func TestUsersPageShowsSavedRolesSelected(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleManage})
	h, _, _ := usersPanel(t, []webauth.User{ann}, []string{"blog", "shop"})
	c, _ := adminCookie(t, h)
	w := do(h, "GET", "/users", nil, cookie(c))
	if w.Code != 200 {
		t.Fatalf("GET /users: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `<option value="manage" selected>`) {
		t.Error("the saved role of blog is not selected")
	}
	if !strings.Contains(body, `<option value="" selected>`) {
		t.Error("shop, which ann has no role on, does not start at No access")
	}
	if !strings.Contains(body, `id="user-ann"`) || !strings.Contains(body, "blog (") {
		t.Error("the user's row or its websites summary is missing")
	}
}
