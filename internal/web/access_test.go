package web

import (
	"net/http"
	"sort"
	"testing"
)

// patternRecorder stands in for *http.ServeMux and keeps the pattern of each route it is given.
type patternRecorder struct{ pats []string }

func (p *patternRecorder) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	p.pats = append(p.pats, pattern)
}

// registeredPatterns lists every pattern the routes register, by running the real registrars against a recorder.
func registeredPatterns(t *testing.T) []string {
	t.Helper()
	rec := &patternRecorder{}
	s := newServer()
	s.NotifySocket = "/nonexistent.sock" // so the hub relay routes are registered too
	s.routes(rec)
	return rec.pats
}

// TestEveryRouteHasARule fails when a registered route is neither public nor given a rule, so a route added later cannot
// slip past the access table. It also fails when the table names a route that is not registered, so renames are caught.
func TestEveryRouteHasARule(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range registeredPatterns(t) {
		seen[p] = true
		if isPublic(p) {
			continue
		}
		if _, ok := accessRules[p]; !ok {
			t.Errorf("route %q has no access rule; add it to accessRules in access.go", p)
		}
	}
	for p := range accessRules {
		if !seen[p] {
			t.Errorf("access rule %q names a route that is not registered", p)
		}
	}
	for p := range publicRoutes {
		if !seen[p] {
			t.Errorf("public route %q is not registered", p)
		}
	}
}

// TestRulesAreWellFormed checks that every rule names a valid source and a role that exists.
func TestRulesAreWellFormed(t *testing.T) {
	keys := make([]string, 0, len(accessRules))
	for k := range accessRules {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rl := accessRules[k]
		if rl.admin {
			continue
		}
		switch rl.src {
		case "":
			own := map[string]bool{
				"POST /logout": true, "GET /result": true, "GET /{$}": true,
				"GET /settings": true, "GET /settings/2fa": true, "POST /settings/password": true,
				"POST /settings/2fa/enable": true, "POST /settings/2fa/disable": true, "POST /settings/sessions/end": true,
			}
			if !own[k] {
				t.Errorf("%s: a signed-in-only rule must be one of the own pages", k)
			}
		case "path", "post", "query", "db":
			if !rl.min.Valid() {
				t.Errorf("%s: rule has no valid role", k)
			}
		default:
			t.Errorf("%s: unknown source %q", k, rl.src)
		}
	}
}
