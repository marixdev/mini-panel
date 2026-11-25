package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mpn/internal/audit"
	"mpn/internal/ctl"
	"mpn/internal/webauth"
)

// activityAnswer answers audit.list from a fixed set of entries, filtered by the website the request names. An entry with no
// website is returned only when the request names no website, as the helper does.
func activityAnswer(entries []audit.Entry) func(ctl.Request) (ctl.Response, error) {
	return func(r ctl.Request) (ctl.Response, error) {
		switch r.Op {
		case "site.list":
			return ctl.Response{OK: true, Output: `[{"name":"blog"},{"name":"shop"}]`}, nil
		case "audit.list":
			var out []audit.Entry
			for _, e := range entries {
				if r.Site == "" || e.Site == r.Site {
					out = append(out, e)
				}
			}
			b, _ := json.Marshal(audit.Page{Entries: out, Total: len(out)})
			return ctl.Response{OK: true, Output: string(b)}, nil
		}
		return ctl.Response{OK: true, Output: "[]"}, nil
	}
}

var activityLog = []audit.Entry{
	{ID: 1, At: 1000, Site: "blog", Op: "file.save", Kind: "files", OK: true, Detail: "blog-entry"},
	{ID: 2, At: 1001, Site: "shop", Op: "file.save", Kind: "files", OK: true, Detail: "shop-entry"},
	{ID: 3, At: 1002, Site: "", Op: "firewall.enable", Kind: "security", OK: true, Detail: "server-entry"},
	{ID: 4, At: 1003, Site: "other", Op: "file.save", Kind: "files", OK: true, Detail: "other-entry"},
}

// activityPanel returns a panel for the scenario of these tests: ann holds a role on blog only, and the helper answers from
// activityLog.
func activityPanel(t *testing.T) (http.Handler, *Server) {
	t.Helper()
	ann := userWith(t, "ann", map[string]webauth.Role{"blog": webauth.RoleView})
	h, s, f := delegatedPanel(t, []webauth.User{ann}, []string{"blog", "shop"})
	f.answer = activityAnswer(activityLog)
	return h, s
}

func TestActivityWithoutSiteIsFilteredToHeldSites(t *testing.T) {
	h, _ := activityPanel(t)
	c := signIn(t, h, "ann", delegatedPW)
	w := do(h, "GET", "/activity", nil, cookie(c))
	if w.Code != 200 {
		t.Fatalf("activity without site: %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "blog-entry") {
		t.Errorf("the held website's entry is missing from the list")
	}
	for _, hidden := range []string{"shop-entry", "other-entry", "server-entry"} {
		if strings.Contains(body, hidden) {
			t.Errorf("%s is shown to a user without a role on its website", hidden)
		}
	}
}

func TestActivityNamingAnotherSiteIsRefused(t *testing.T) {
	h, _ := activityPanel(t)
	c := signIn(t, h, "ann", delegatedPW)
	for _, p := range []string{"/activity?site=shop", "/activity.csv?site=shop", "/activity?site=nosuch", "/activity.csv?site=nosuch"} {
		if w := do(h, "GET", p, nil, cookie(c)); w.Code != 403 {
			t.Errorf("%s: %d, want 403", p, w.Code)
		}
	}
}

func TestActivityCSVWithoutSiteIsFilteredLikeTheList(t *testing.T) {
	h, _ := activityPanel(t)
	c := signIn(t, h, "ann", delegatedPW)
	w := do(h, "GET", "/activity.csv", nil, cookie(c))
	if w.Code != 200 {
		t.Fatalf("csv without site: %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "blog-entry") {
		t.Errorf("csv is missing the held website's entry")
	}
	for _, hidden := range []string{"shop-entry", "other-entry", "server-entry"} {
		if strings.Contains(body, hidden) {
			t.Errorf("csv shows %s to a user without a role on its website", hidden)
		}
	}
}

func TestActivityAdministratorSeesEverything(t *testing.T) {
	h, _, c, _ := setup(t, activityAnswer(activityLog))
	body := do(h, "GET", "/activity", nil, cookie(c)).Body.String()
	for _, want := range []string{"blog-entry", "shop-entry", "server-entry", "other-entry"} {
		if !strings.Contains(body, want) {
			t.Errorf("administrator does not see %s", want)
		}
	}
}
