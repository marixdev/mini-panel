package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mpn/internal/webauth"
)

// usersPanel is delegatedPanel with the administrator's file path returned, so tests can read what an action saved.
func usersPanel(t *testing.T, users []webauth.User, liveSites []string) (http.Handler, *Server, string) {
	t.Helper()
	h, s, _ := delegatedPanel(t, users, liveSites)
	return h, s, s.UsersFile
}

// adminCookie signs in as the administrator and returns the cookie and form token of the sites page.
func adminCookie(t *testing.T, h http.Handler) (*http.Cookie, string) {
	t.Helper()
	c := cookieNamed(login(h, goodPW), "mpn")
	if c == nil {
		t.Fatal("administrator could not sign in")
	}
	return c, csrfOf(t, h, c)
}

// loadFile reads the users file as the panel does, so each test checks what was actually saved.
func loadFile(t *testing.T, path string) []webauth.User {
	t.Helper()
	users, err := webauth.LoadUsers(path, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return users
}

func findIn(users []webauth.User, name string) (webauth.User, bool) {
	for _, u := range users {
		if u.Name == name {
			return u, true
		}
	}
	return webauth.User{}, false
}

func TestUsersPageIsAdminOnly(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, _ := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	if w := do(h, "GET", "/users", nil, cookie(c)); w.Code != 403 {
		t.Errorf("delegated user opened /users: %d, want 403", w.Code)
	}
	ac, _ := adminCookie(t, h)
	if w := do(h, "GET", "/users", nil, cookie(ac)); w.Code != 200 {
		t.Errorf("administrator /users: %d, want 200", w.Code)
	}
}

func TestCreateUserSavesAndCanSignIn(t *testing.T) {
	h, _, path := usersPanel(t, nil, []string{"blog"})
	ac, tok := adminCookie(t, h)
	form := url.Values{"csrf": {tok}, "name": {"dan"}, "password": {delegatedPW}, "again": {delegatedPW}}
	if w := do(h, "POST", "/users/create", form, cookie(ac)); w.Code != 303 {
		t.Fatalf("create: %d, want 303", w.Code)
	}
	u, ok := findIn(loadFile(t, path), "dan")
	if !ok {
		t.Fatal("created user is not in the file")
	}
	if u.Hash == "" || u.Hash == delegatedPW {
		t.Error("password was not stored as a hash")
	}
	if c := signIn(t, h, "dan", delegatedPW); c == nil {
		t.Error("new user cannot sign in")
	}
}

func TestCreateUserRefusesBadInput(t *testing.T) {
	h, _, path := usersPanel(t, nil, []string{"blog"})
	ac, tok := adminCookie(t, h)
	cases := map[string]url.Values{
		"bad name":       {"csrf": {tok}, "name": {"Bad Name"}, "password": {delegatedPW}, "again": {delegatedPW}},
		"short password": {"csrf": {tok}, "name": {"eve"}, "password": {"short"}, "again": {"short"}},
		"mismatch":       {"csrf": {tok}, "name": {"eve"}, "password": {delegatedPW}, "again": {"other-password-here"}},
		"administrator":  {"csrf": {tok}, "name": {"admin"}, "password": {delegatedPW}, "again": {delegatedPW}},
	}
	for name, form := range cases {
		do(h, "POST", "/users/create", form, cookie(ac))
		if len(loadFile(t, path)) != 0 {
			t.Errorf("%s: a user was saved", name)
		}
	}
}

func TestCreateUserRefusesADuplicate(t *testing.T) {
	ann := userWith(t, "ann", nil)
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	ac, tok := adminCookie(t, h)
	do(h, "POST", "/users/create", url.Values{"csrf": {tok}, "name": {"ann"}, "password": {delegatedPW}, "again": {delegatedPW}}, cookie(ac))
	if n := len(loadFile(t, path)); n != 1 {
		t.Errorf("duplicate name: %d users, want 1", n)
	}
}

func TestSetSitesSavesRolesAndDropsRemovedSites(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"gone": webauth.RoleFull})
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"blog", "shop"})
	ac, tok := adminCookie(t, h)
	form := url.Values{"csrf": {tok}, "name": {"ann"}, "site.blog": {"view"}, "site.shop": {"manage"}}
	if w := do(h, "POST", "/users/sites", form, cookie(ac)); w.Code != 303 {
		t.Fatalf("sites: %d, want 303", w.Code)
	}
	u, _ := findIn(loadFile(t, path), "ann")
	if u.RoleOn("blog") != webauth.RoleView || u.RoleOn("shop") != webauth.RoleManage {
		t.Errorf("roles not saved: %+v", u.Sites)
	}
	if u.RoleOn("gone") != "" {
		t.Error("a role on a removed site was kept")
	}
}

func TestSetSitesRefusesAnUnknownRole(t *testing.T) {
	ann := userWith(t, "ann", nil)
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	ac, tok := adminCookie(t, h)
	do(h, "POST", "/users/sites", url.Values{"csrf": {tok}, "name": {"ann"}, "site.blog": {"root"}}, cookie(ac))
	u, _ := findIn(loadFile(t, path), "ann")
	if len(u.Sites) != 0 {
		t.Errorf("an invalid role was saved: %+v", u.Sites)
	}
}

func TestPasswordResetEndsSessions(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, _ := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	old := signIn(t, h, "ann", delegatedPW)
	ac, tok := adminCookie(t, h)
	form := url.Values{"csrf": {tok}, "name": {"ann"}, "password": {"new-horse-battery"}, "again": {"new-horse-battery"}}
	if w := do(h, "POST", "/users/password", form, cookie(ac)); w.Code != 303 {
		t.Fatalf("reset: %d, want 303", w.Code)
	}
	if w := do(h, "GET", "/sites/blog", nil, cookie(old)); w.Code == 200 {
		t.Error("the old session still works after a password reset")
	}
	if c := signIn(t, h, "ann", "new-horse-battery"); c == nil {
		t.Error("new password does not sign in")
	}
	if w := do(h, "POST", "/login", url.Values{"user": {"ann"}, "password": {delegatedPW}}); w.Code != 401 {
		t.Errorf("old password still works: %d", w.Code)
	}
}

func TestDisableEndsSessionsAndEnableRestores(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	ac, tok := adminCookie(t, h)
	do(h, "POST", "/users/disable", url.Values{"csrf": {tok}, "name": {"ann"}}, cookie(ac))
	if u, _ := findIn(loadFile(t, path), "ann"); !u.Disabled {
		t.Fatal("user was not disabled in the file")
	}
	if w := do(h, "GET", "/sites/blog", nil, cookie(c)); w.Code == 200 {
		t.Error("a disabled user's session still works")
	}
	if w := do(h, "POST", "/login", url.Values{"user": {"ann"}, "password": {delegatedPW}}); w.Code != 401 {
		t.Errorf("disabled user signed in: %d", w.Code)
	}
	do(h, "POST", "/users/enable", url.Values{"csrf": {tok}, "name": {"ann"}}, cookie(ac))
	if u, _ := findIn(loadFile(t, path), "ann"); u.Disabled {
		t.Error("user was not enabled again")
	}
	if c := signIn(t, h, "ann", delegatedPW); c == nil {
		t.Error("re-enabled user cannot sign in")
	}
}

func TestRemoveNeedsTheNameTypedAndEndsSessions(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	ac, tok := adminCookie(t, h)
	do(h, "POST", "/users/remove", url.Values{"csrf": {tok}, "name": {"ann"}, "confirm": {"wrong"}}, cookie(ac))
	if _, ok := findIn(loadFile(t, path), "ann"); !ok {
		t.Fatal("user removed without the name typed")
	}
	do(h, "POST", "/users/remove", url.Values{"csrf": {tok}, "name": {"ann"}, "confirm": {"ann"}}, cookie(ac))
	if _, ok := findIn(loadFile(t, path), "ann"); ok {
		t.Error("user still in the file after removal")
	}
	if w := do(h, "GET", "/sites/blog", nil, cookie(c)); w.Code == 200 {
		t.Error("a removed user's session still works")
	}
}

func TestTwoFAOffClearsTheSecretAndEndsSessions(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	// The secret is set after the sign-in, so the sign-in above is a plain one; the action then clears the secret.
	ac0, tok0 := adminCookie(t, h)
	saved := loadFile(t, path)
	for i := range saved {
		if saved[i].Name == "ann" {
			saved[i].TOTP = "JBSWY3DPEHPK3PXP"
		}
	}
	if err := webauth.SaveUsers(path, saved, "admin", -1, -1); err != nil {
		t.Fatal(err)
	}
	_, _ = ac0, tok0
	ac, tok := adminCookie(t, h)
	do(h, "POST", "/users/2fa-off", url.Values{"csrf": {tok}, "name": {"ann"}}, cookie(ac))
	u, _ := findIn(loadFile(t, path), "ann")
	if u.TOTP != "" {
		t.Error("the two-step secret was kept")
	}
	if w := do(h, "GET", "/sites/blog", nil, cookie(c)); w.Code == 200 {
		t.Error("the session survived turning two-step off")
	}
}

func TestPMAFlagSetsAndTakesAwayAndEndsSessions(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"shop": webauth.RoleManage})
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"shop"})
	c := signIn(t, h, "ann", delegatedPW)
	ac, tok := adminCookie(t, h)
	do(h, "POST", "/users/pma", url.Values{"csrf": {tok}, "name": {"ann"}, "on": {"1"}}, cookie(ac))
	if u, _ := findIn(loadFile(t, path), "ann"); !u.PMA {
		t.Fatal("flag was not turned on")
	}
	do(h, "POST", "/users/pma", url.Values{"csrf": {tok}, "name": {"ann"}, "on": {"0"}}, cookie(ac))
	if u, _ := findIn(loadFile(t, path), "ann"); u.PMA {
		t.Error("flag was not turned off")
	}
	if w := do(h, "GET", "/sites/shop", nil, cookie(c)); w.Code == 200 {
		t.Error("the session survived taking away the database browser")
	}
}

func TestUsersActionsNeedAPostFormToken(t *testing.T) {
	ann := userWith(t, "ann", nil)
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	ac, _ := adminCookie(t, h)
	do(h, "POST", "/users/disable", url.Values{"name": {"ann"}}, cookie(ac))
	if u, _ := findIn(loadFile(t, path), "ann"); u.Disabled {
		t.Error("an action without the form token was applied")
	}
}

// An unreadable users file refuses every change, so an action never overwrites what it could not read.
func TestBrokenUsersFileIsNeverOverwritten(t *testing.T) {
	ann := userWith(t, "ann", nil)
	h, _, path := usersPanel(t, []webauth.User{ann}, []string{"blog"})
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	ac, tok := adminCookie(t, h)
	do(h, "POST", "/users/create", url.Values{"csrf": {tok}, "name": {"dan"}, "password": {delegatedPW}, "again": {delegatedPW}}, cookie(ac))
	b, _ := os.ReadFile(filepath.Clean(path))
	if strings.TrimSpace(string(b)) != "{not json" {
		t.Errorf("a broken users file was overwritten: %q", b)
	}
}
