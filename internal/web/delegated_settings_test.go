package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mpn/internal/webauth"
)

// A delegated user sees only the password, two-step sign-in and sessions of their own account on /settings, and changes them with
// their own record in the users file. Every other settings route stays administrator only.
func TestDelegatedSettingsChangesOwnPasswordOnly(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	tok := csrfOf(t, h, c)

	page := do(h, "GET", "/settings", nil, cookie(c)).Body.String()
	if !strings.Contains(page, `action="/settings/password"`) {
		t.Error("delegated user does not see the password form")
	}
	if strings.Contains(page, "mpn web 2fa-off") {
		t.Error("delegated user sees the administrator's recovery command")
	}

	if w := post(h, "/settings/password", tok, c, "current", "wrong-password-here", "new", "another-long-password", "again", "another-long-password"); w.StatusCode != 401 {
		t.Errorf("wrong current password: %d, want 401", w.StatusCode)
	}
	if w := post(h, "/settings/password", tok, c, "current", delegatedPW, "new", "another-long-password", "again", "another-long-password"); w.StatusCode != 303 {
		t.Errorf("right current password: %d, want 303", w.StatusCode)
	}
	// The new password works, the old one does not.
	if w := do(h, "POST", "/login", url.Values{"user": {"ann"}, "password": {delegatedPW}}); w.Code != 401 {
		t.Errorf("old password still signs in: %d", w.Code)
	}
	if c2 := signIn(t, h, "ann", "another-long-password"); c2 == nil {
		t.Error("new password does not sign in")
	}
}

// Ending the other sessions of a delegated user keeps the browser that ends them, and leaves the administrator's sessions alone.
func TestDelegatedSessionsEndKeepsOnlyThisBrowser(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	other := signIn(t, h, "ann", delegatedPW)
	admin := login(h, goodPW)
	adminCookie := cookieNamed(admin, "mpn")
	tok := csrfOf(t, h, c)

	if w := post(h, "/settings/sessions/end", tok, c); w.StatusCode != 303 {
		t.Fatalf("end other sessions: %d, want 303", w.StatusCode)
	}
	if w := do(h, "GET", "/sites/blog", nil, cookie(other)); w.Code == 200 {
		t.Error("the other browser of ann is still signed in")
	}
	if w := do(h, "GET", "/sites/blog", nil, cookie(c)); w.Code != 200 {
		t.Errorf("this browser lost its session: %d", w.Code)
	}
	if w := do(h, "GET", "/settings", nil, cookie(adminCookie)); w.Code != 200 {
		t.Errorf("the administrator was signed out by a delegated user: %d", w.Code)
	}
}

// Every other settings route is still refused to a delegated user.
func TestDelegatedUserIsRefusedAdminOnlySettingsRoutes(t *testing.T) {
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleFull})
	h, _, _ := delegatedPanel(t, []webauth.User{ann}, []string{"blog"})
	c := signIn(t, h, "ann", delegatedPW)
	tok := csrfOf(t, h, c)

	if w := do(h, "GET", "/update", nil, cookie(c)); w.Code != 403 {
		t.Errorf("GET /update: %d, want 403", w.Code)
	}
	if w := post(h, "/update", tok, c, "version", "1.0.0"); w.StatusCode != 403 {
		t.Errorf("POST /update: %d, want 403", w.StatusCode)
	}
	if w := post(h, "/settings/update", tok, c, "version", "1.0.0"); w.StatusCode != 403 {
		t.Errorf("POST /settings/update: %d, want 403", w.StatusCode)
	}
	if w := do(h, "GET", "/settings", nil, cookie(c)); w.Code != 200 || !strings.Contains(w.Body.String(), `action="/settings/password"`) {
		t.Errorf("GET /settings for a delegated user: %d", w.Code)
	}
	_ = http.StatusOK
}
