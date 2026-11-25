package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mpn/internal/ctl"
	"mpn/internal/webauth"
)

const delegatedPW = "delegated-horse-battery"

// delegatedPanel returns a panel with a users file holding the given delegated users, and a fake helper that knows the sites
// listed in liveSites. The administrator is the usual test account.
func delegatedPanel(t *testing.T, users []webauth.User, liveSites []string) (http.Handler, *Server, *fakeHelper) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.json")
	if err := webauth.SaveUsers(path, users, "admin", -1, -1); err != nil {
		t.Fatal(err)
	}
	rows := "["
	for i, n := range liveSites {
		if i > 0 {
			rows += ","
		}
		rows += `{"name":"` + n + `"}`
	}
	rows += "]"
	f := &fakeHelper{answer: okAnswer(rows)}
	s := newServer()
	s.UsersFile = path
	s.Call = f.call
	return s.Handler(), s, f
}

func userWith(t *testing.T, name string, sites map[string]webauth.Role) webauth.User {
	t.Helper()
	h, err := webauth.Hash(delegatedPW)
	if err != nil {
		t.Fatal(err)
	}
	return webauth.User{Name: name, Hash: h, Sites: sites}
}

// signIn logs a user in and returns the session cookie.
func signIn(t *testing.T, h http.Handler, name, pw string) *http.Cookie {
	t.Helper()
	w := do(h, "POST", "/login", url.Values{"user": {name}, "password": {pw}})
	c := cookieNamed(w, "mpn")
	if c == nil {
		t.Fatalf("sign-in as %s: %d, no cookie", name, w.Code)
	}
	return c
}

// csrfOf returns the form token of the page a signed-in browser sees.
func csrfOf(t *testing.T, h http.Handler, c *http.Cookie) string {
	t.Helper()
	m := csrfRe.FindStringSubmatch(do(h, "GET", "/", nil, cookie(c)).Body.String())
	if m == nil {
		t.Fatal("no form token on the sites page")
	}
	return m[1]
}

func TestDelegatedUserSignsInAndAdminIsUnaffected(t *testing.T) {
	u := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, _ := delegatedPanel(t, []webauth.User{u}, []string{"blog"})
	if w := do(h, "POST", "/login", url.Values{"user": {"ann"}, "password": {"wrong"}}); w.Code != 401 {
		t.Errorf("wrong password: %d, want 401", w.Code)
	}
	if c := cookieNamed(login(h, goodPW), "mpn"); c == nil {
		t.Error("the administrator no longer signs in")
	}
	if c := signIn(t, h, "ann", delegatedPW); c == nil {
		t.Error("delegated user did not sign in")
	}
}

func TestDisabledDelegatedUserCannotSignIn(t *testing.T) {
	u := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	u.Disabled = true
	h, _, _ := delegatedPanel(t, []webauth.User{u}, []string{"blog"})
	if w := do(h, "POST", "/login", url.Values{"user": {"ann"}, "password": {delegatedPW}}); w.Code != 401 {
		t.Errorf("disabled user: %d, want 401", w.Code)
	}
}

func TestUnknownNameStillTakesThePasswordCheck(t *testing.T) {
	u := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, _ := delegatedPanel(t, []webauth.User{u}, []string{"blog"})
	if w := do(h, "POST", "/login", url.Values{"user": {"nobody"}, "password": {delegatedPW}}); w.Code != 401 {
		t.Errorf("unknown name: %d, want 401", w.Code)
	}
}

// The role table: each request is one that the role either allows or refuses on an assigned and an unassigned website.
func TestRolesAllowAndRefuseByWebsite(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView, "shop": webauth.RoleManage, "news": webauth.RoleFull})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"blog", "shop", "news", "other"})
	c := signIn(t, h, "ann", delegatedPW)
	tok := csrfOf(t, h, c)

	cases := []struct {
		name, method, target string
		form                 url.Values
		want                 int
	}{
		{"view overview of own site", "GET", "/sites/blog", nil, 200},
		{"view overview of site not given", "GET", "/sites/other", nil, 403},
		{"view cannot edit a file", "GET", "/sites/blog/files/edit?path=x", nil, 403},
		{"view cannot save a file", "POST", "/sites/blog/files/save", url.Values{"csrf": {tok}}, 403},
		{"manage cannot open terminal", "GET", "/sites/shop/terminal", nil, 403},
		{"full can open terminal", "GET", "/sites/news/terminal", nil, 200},
		{"manage cannot change domain", "POST", "/sites/shop/domain", url.Values{"csrf": {tok}}, 403},
		{"full can change domain", "POST", "/sites/news/domain", url.Values{"csrf": {tok}, "domain": {"news.example.com"}}, 303},
		{"a server page is administrator only", "GET", "/firewall", nil, 403},
		{"a server page with a POST", "POST", "/firewall/enable", url.Values{"csrf": {tok}}, 403},
		{"site creation is administrator only", "GET", "/sites/new", nil, 403},
		{"an unknown route is refused", "GET", "/no/such/page", nil, 404},
	}
	for _, tc := range cases {
		var form url.Values
		if tc.form != nil {
			form = tc.form
		}
		w := do(h, tc.method, tc.target, form, cookie(c))
		if w.Code != tc.want {
			t.Errorf("%s: %s %s = %d, want %d", tc.name, tc.method, tc.target, w.Code, tc.want)
		}
	}
}

// A website that is removed grants nothing, even though the role remains in the users file.
func TestRoleOnARemovedSiteGrantsNothing(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"gone": webauth.RoleFull})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	if w := do(h, "GET", "/sites/gone/terminal", nil, cookie(c)); w.Code != 403 {
		t.Errorf("removed site: %d, want 403", w.Code)
	}
}

// Database actions check the website that owns the database, not whatever website the form names.
func TestDatabaseActionsNeedTheOwningSite(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"shop": webauth.RoleManage})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"shop", "blog"})
	c := signIn(t, h, "ann", delegatedPW)
	tok := csrfOf(t, h, c)
	if w := do(h, "POST", "/databases/passwd", url.Values{"csrf": {tok}, "name": {"blog_main"}, "site": {"shop"}}, cookie(c)); w.Code != 403 {
		t.Errorf("a blog database changed through a shop form: %d, want 403", w.Code)
	}
	if w := do(h, "POST", "/databases/passwd", url.Values{"csrf": {tok}, "name": {"shop_main"}, "site": {"shop"}}, cookie(c)); w.Code == 403 {
		t.Errorf("a shop database refused: %d", w.Code)
	}
}

// Restoring a backup needs the full role; manage may only create one.
func TestRestoreNeedsFull(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"shop": webauth.RoleManage})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"shop"})
	c := signIn(t, h, "ann", delegatedPW)
	tok := csrfOf(t, h, c)
	if w := do(h, "POST", "/backups/restore", url.Values{"csrf": {tok}, "site": {"shop"}}, cookie(c)); w.Code != 403 {
		t.Errorf("restore by manage: %d, want 403", w.Code)
	}
	if w := do(h, "POST", "/backups/create", url.Values{"csrf": {tok}, "site": {"shop"}}, cookie(c)); w.Code == 403 {
		t.Errorf("create by manage refused: %d", w.Code)
	}
}

// Activity is narrowed to the websites a user holds a role on; a site the user lacks is refused.
func TestActivityForDelegatedUserIsRefusedForOtherSites(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"blog", "shop"})
	c := signIn(t, h, "ann", delegatedPW)
	if w := do(h, "GET", "/activity?site=shop", nil, cookie(c)); w.Code != 403 {
		t.Errorf("activity of a site not given: %d, want 403", w.Code)
	}
	if w := do(h, "GET", "/activity?site=blog", nil, cookie(c)); w.Code != 200 {
		t.Errorf("activity of an assigned site: %d, want 200", w.Code)
	}
}

// Uploads go through the same check as every other route: a user with no role on the site cannot upload to it.
func TestUploadNeedsManageOnTheSite(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"blog", "shop"})
	c := signIn(t, h, "ann", delegatedPW)
	tok := csrfOf(t, h, c)
	target := "/sites/shop/files/upload?csrf=" + url.QueryEscape(tok)
	if w := do(h, "POST", target, nil, cookie(c)); w.Code != 403 {
		t.Errorf("upload to a site not given: %d, want 403", w.Code)
	}
}

// The administrator's password change ends the delegated user's sessions.
func TestDisableEndsDelegatedSessions(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, s, _ := delegatedPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	if w := do(h, "GET", "/sites/blog", nil, cookie(c)); w.Code != 200 {
		t.Fatalf("before the change: %d", w.Code)
	}
	s.Sessions.DeleteUser("ann")
	if w := do(h, "GET", "/sites/blog", nil, cookie(c)); w.Code != 303 {
		t.Errorf("after the user was ended: %d, want a redirect to sign in", w.Code)
	}
}

func TestDelegatedNameCannotBeTheAdministrator(t *testing.T) {
	if err := webauth.ValidateUsers([]webauth.User{{Name: "Admin", Hash: "x"}}, "admin"); err == nil {
		t.Error("a delegated user named like the administrator was accepted")
	}
}

var _ = strings.Contains
var _ = ctl.Request{}

// startDelegatedFM serves the panel over HTTP with a fake SSH server behind it, and a delegated user "ann" with the given role
// on the blog website. It returns an environment signed in as ann, with the form token of the sites page.
func startDelegatedFM(t *testing.T, role webauth.Role) fmEnv {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.json")
	if err := webauth.SaveUsers(path, []webauth.User{userWith(t, "ann", map[string]webauth.Role{"blog": role})}, "admin", -1, -1); err != nil {
		t.Fatal(err)
	}
	e := startTerminalEnv(t, func(s *Server) { s.UsersFile = path })
	home := filepath.FromSlash(e.ssh.Home)
	if err := os.WriteFile(filepath.Join(home, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	anon := fmEnv{termEnv: e, home: home}
	anon.cookie = ""
	resp, _ := anon.post(t, "/login", url.Values{"user": {"ann"}, "password": {delegatedPW}})
	var c *http.Cookie
	for _, k := range resp.Cookies() {
		if k.Name == "mpn" {
			c = k
		}
	}
	if c == nil {
		t.Fatalf("delegated sign-in: %d, no cookie", resp.StatusCode)
	}
	de := fmEnv{termEnv: e, home: home}
	de.cookie = c.Name + "=" + c.Value
	_, page := de.get(t, "/")
	m := csrfRe.FindStringSubmatch(page)
	if m == nil {
		t.Fatal("no form token on the sites page")
	}
	de.token = m[1]
	return de
}

// A manage user saves through the real SFTP path: the redirect is the success, and the file on the fake server changes.
func TestDelegatedManageUserSavesAFile(t *testing.T) {
	de := startDelegatedFM(t, webauth.RoleManage)
	mt := mtimeOf(t, filepath.Join(de.home, "notes.txt"))
	resp, body := de.post(t, "/sites/blog/files/save", url.Values{"csrf": {de.token}, "path": {"notes.txt"}, "mtime": {mt}, "eol": {"lf"}, "content": {"changed by manage"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("manage save: %d, want 303\n%s", resp.StatusCode, body)
	}
	if got := de.read(t, "notes.txt"); got != "changed by manage" {
		t.Errorf("file after manage save = %q", got)
	}
}

// A view user is refused before any SFTP login, and the file keeps its content.
func TestDelegatedViewUserCannotSaveAFile(t *testing.T) {
	de := startDelegatedFM(t, webauth.RoleView)
	mt := mtimeOf(t, filepath.Join(de.home, "notes.txt"))
	resp, _ := de.post(t, "/sites/blog/files/save", url.Values{"csrf": {de.token}, "path": {"notes.txt"}, "mtime": {mt}, "eol": {"lf"}, "content": {"changed by view"}})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("view save: %d, want 403", resp.StatusCode)
	}
	if got := de.read(t, "notes.txt"); got != "hello" {
		t.Errorf("file after refused save = %q, want unchanged", got)
	}
}

// The database browser needs the administrator's flag, and manage on the website that owns the database.
func TestPMAAccessNeedsTheFlagAndManageOnTheOwner(t *testing.T) {
	off := userWith(t, "ann", map[string]webauth.Role{"shop": webauth.RoleManage})
	on := userWith(t, "bob", map[string]webauth.Role{"shop": webauth.RoleManage})
	on.PMA = true
	noRole := userWith(t, "cat", map[string]webauth.Role{"blog": webauth.RoleManage})
	noRole.PMA = true
	h, _, _ := delegatedPanel(t, []webauth.User{off, on, noRole}, []string{"shop", "blog"})

	try := func(user string) int {
		c := signIn(t, h, user, delegatedPW)
		tok := csrfOf(t, h, c)
		return do(h, "POST", "/databases/pma", url.Values{"csrf": {tok}, "name": {"shop_main"}}, cookie(c)).Code
	}
	if got := try("ann"); got != 403 {
		t.Errorf("flag off: %d, want 403", got)
	}
	if got := try("bob"); got == 403 {
		t.Errorf("flag on with manage on the owner was refused")
	}
	if got := try("cat"); got != 403 {
		t.Errorf("flag on but no role on the owning site: %d, want 403", got)
	}
}
