package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"mpn/internal/webauth"
)

// hidingHelper answers the operations the site and server pages use: the backup answers, with a site list that names one
// website, blog, so the delegated users below hold a role on it.
func hidingHelper() map[string]string {
	m := backupHelper()
	m["site.list"] = `[{"name":"blog","kind":"php","user":"blog","hostnames":["blog.example.com"],"detail":"php 8.4","tls":true}]`
	m["site.info"] = phpInfo()
	m["site.sslinfo"] = `{}`
	return m
}

// hidingPanel is delegatedPanel with a helper that answers each operation separately, so the site pages render.
func hidingPanel(t *testing.T, users []webauth.User) http.Handler {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.json")
	if err := webauth.SaveUsers(path, users, "admin", -1, -1); err != nil {
		t.Fatal(err)
	}
	f := &fakeHelper{answer: byOp(hidingHelper())}
	s := newServer()
	s.UsersFile = path
	s.Call = f.call
	return s.Handler()
}

// hidingSessions signs in one view, one manage and one full user on blog, and the administrator. The map is keyed by role.
func hidingSessions(t *testing.T) (http.Handler, map[string]*http.Cookie) {
	t.Helper()
	view := userWith(t, "vic", map[string]webauth.Role{"blog": webauth.RoleView})
	manage := userWith(t, "max", map[string]webauth.Role{"blog": webauth.RoleManage})
	full := userWith(t, "fay", map[string]webauth.Role{"blog": webauth.RoleFull})
	h := hidingPanel(t, []webauth.User{view, manage, full})
	return h, map[string]*http.Cookie{
		"view":   signIn(t, h, "vic", delegatedPW),
		"manage": signIn(t, h, "max", delegatedPW),
		"full":   signIn(t, h, "fay", delegatedPW),
		"admin":  cookieNamed(login(h, goodPW), "mpn"),
	}
}

// hidingPage fetches a page as the signed-in browser and fails the test unless the page answers 200.
func hidingPage(t *testing.T, h http.Handler, c *http.Cookie, path string) string {
	t.Helper()
	w := do(h, "GET", path, nil, cookie(c))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: %d", path, w.Code)
	}
	return w.Body.String()
}

// wantPresent and wantAbsent check the rendered HTML for each string. who names the person for the messages.
func wantPresent(t *testing.T, who, body string, items ...string) {
	t.Helper()
	for _, it := range items {
		if !strings.Contains(body, it) {
			t.Errorf("%s should see %q", who, it)
		}
	}
}

func wantAbsent(t *testing.T, who, body string, items ...string) {
	t.Helper()
	for _, it := range items {
		if strings.Contains(body, it) {
			t.Errorf("%s should not see %q", who, it)
		}
	}
}

// The server-level navigation: Sites and Activity for everyone, the rest for the administrator only.
const (
	navSites    = `href="/" class="nav-link"`
	navActivity = `href="/activity" class="nav-link"`
	navFirewall = `href="/firewall" class="nav-link"`
	navMonitor  = `href="/monitor" class="nav-link"`
	navUpdate   = `href="/update" class="nav-link"`
	navNotify   = `href="/notify" class="nav-link"`
	navSitesNew = `href="/sites/new"`
)

func TestViewUserSeesReadOnlyContentAndNoManageControls(t *testing.T) {
	h, c := hidingSessions(t)
	vic := c["view"]

	overview := hidingPage(t, h, vic, "/sites/blog")
	wantPresent(t, "view user on the overview", overview, "blog.example.com")
	wantAbsent(t, "view user on the overview", overview,
		`action="/sites/blog/speed"`, `action="/sites/blog/webmail"`, `action="/sites/blog/app/install"`,
		`action="/sites/blog/ssh/enable"`, `href="/sites/remove?site=blog"`, `href="/sites/blog/terminal"`)

	backups := hidingPage(t, h, vic, "/sites/blog/backups")
	wantPresent(t, "view user on backups", backups, "1.5 MB")
	wantAbsent(t, "view user on backups", backups, `action="/backups/create"`, `action="/backups/keep"`,
		"/backups/restore?site=blog&amp;backup=20261104-031500")

	cron := hidingPage(t, h, vic, "/sites/blog/cron")
	wantAbsent(t, "view user on cron", cron, `action="/sites/blog/cron"`)

	ssl := hidingPage(t, h, vic, "/sites/blog/ssl")
	wantAbsent(t, "view user on ssl", ssl, `action="/sites/blog/ssl/letsencrypt"`, `action="/sites/blog/ssl/custom"`,
		`action="/sites/blog/ssl/csr"`)

	home := hidingPage(t, h, vic, "/")
	wantPresent(t, "view user on the sites list", home, navSites, navActivity)
	wantAbsent(t, "view user on the sites list", home, navFirewall, navMonitor, navUpdate, navNotify, navSitesNew)
}

func TestManageUserSeesManageControlsButNotFullOnes(t *testing.T) {
	h, c := hidingSessions(t)
	max := c["manage"]

	overview := hidingPage(t, h, max, "/sites/blog")
	wantPresent(t, "manage user on the overview", overview, `action="/sites/blog/speed"`, `action="/sites/blog/webmail"`,
		`action="/sites/blog/app/install"`)
	wantAbsent(t, "manage user on the overview", overview, `action="/sites/blog/ssh/enable"`,
		`href="/sites/remove?site=blog"`, `href="/sites/blog/terminal"`)

	backups := hidingPage(t, h, max, "/sites/blog/backups")
	wantPresent(t, "manage user on backups", backups, `action="/backups/create"`, `action="/backups/keep"`)
	wantAbsent(t, "manage user on backups", backups, "/backups/restore?site=blog&amp;backup=20261104-031500")

	cron := hidingPage(t, h, max, "/sites/blog/cron")
	wantPresent(t, "manage user on cron", cron, `action="/sites/blog/cron"`)

	ssl := hidingPage(t, h, max, "/sites/blog/ssl")
	wantAbsent(t, "manage user on ssl", ssl, `action="/sites/blog/ssl/letsencrypt"`, `action="/sites/blog/ssl/custom"`)

	home := hidingPage(t, h, max, "/")
	wantPresent(t, "manage user on the sites list", home, navSites, navActivity)
	wantAbsent(t, "manage user on the sites list", home, navFirewall, navMonitor, navUpdate, navNotify, navSitesNew)
}

func TestFullUserSeesRestoreTerminalAndSSLControls(t *testing.T) {
	h, c := hidingSessions(t)
	fay := c["full"]

	backups := hidingPage(t, h, fay, "/sites/blog/backups")
	wantPresent(t, "full user on backups", backups, `action="/backups/create"`, "/backups/restore?site=blog&amp;backup=20261104-031500")

	overview := hidingPage(t, h, fay, "/sites/blog")
	wantPresent(t, "full user on the overview", overview, `href="/sites/blog/terminal"`, `action="/sites/blog/domain"`)
	wantAbsent(t, "full user on the overview", overview, `href="/sites/remove?site=blog"`)

	ssl := hidingPage(t, h, fay, "/sites/blog/ssl")
	// The Let's Encrypt card is shown only for a site without TLS or with its own certificate, and this site has TLS, so only the
	// custom-certificate form is checked here.
	wantPresent(t, "full user on ssl", ssl, `action="/sites/blog/ssl/custom"`)
}

func TestAdminSeesEverything(t *testing.T) {
	h, c := hidingSessions(t)
	adm := c["admin"]

	overview := hidingPage(t, h, adm, "/sites/blog")
	wantPresent(t, "the administrator on the overview", overview, `href="/sites/remove?site=blog"`, `action="/sites/blog/speed"`,
		`href="/sites/blog/terminal"`)

	backups := hidingPage(t, h, adm, "/sites/blog/backups")
	wantPresent(t, "the administrator on backups", backups, `action="/backups/create"`, `action="/backups/keep"`,
		"/backups/restore?site=blog&amp;backup=20261104-031500")

	home := hidingPage(t, h, adm, "/")
	wantPresent(t, "the administrator on the sites list", home, navSites, navActivity, navFirewall, navMonitor, navUpdate,
		navNotify, navSitesNew)
}
