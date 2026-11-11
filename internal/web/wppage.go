package web

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mpn/internal/apps"
	"mpn/internal/ctl"
	"mpn/internal/webauth"
)

type wpItemView struct {
	Name          string `json:"name"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Version       string `json:"version"`
	Update        string `json:"update"`
	UpdateVersion string `json:"update_version"`
}

func (i wpItemView) Active() bool    { return i.Status == "active" || i.Status == "active-network" }
func (i wpItemView) HasUpdate() bool { return i.Update == "available" }

type wpFound struct {
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Rating  any    `json:"rating"`
	Version string `json:"version"`
	Short   string `json:"short_description"`
}

// wpView is what the WordPress tab draws; the first fields are the answer of "mpn wp status".
type wpView struct {
	Version    string       `json:"version"`
	URL        string       `json:"url"`
	CoreUpdate string       `json:"core_update"`
	Plugins    []wpItemView `json:"plugins"`
	Themes     []wpItemView `json:"themes"`

	Error      string    `json:"-"`
	Kind, Term string    `json:"-"` // the search that was asked for
	Found      []wpFound `json:"-"`
	SearchErr  string    `json:"-"`
}

func wpMessage(out string) string {
	return wpMessageText(T, out)
}

// wpMessage turns the refusals of the manager into a sentence for the person; anything else is shown as it is.
func wpMessageText(T translator, out string) string {
	for _, m := range []struct{ marker, key string }{{"wp-notwp", "wp.err.notwp"}, {"wp-broken", "wp.err.broken"}, {"wp-search", "wp.err.search"}} {
		if strings.Contains(out, m.marker) {
			return T(m.key)
		}
	}
	return out
}

func (s *Server) wpRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sites/{name}/wp/run", s.auth(s.wpRun))
}

// loadWP reads the state of the WordPress of a site, and the result of a search when the address carries one (?k=plugin&q=seo).
func (s *Server) loadWP(row siteRow, r *http.Request) *wpView {
	T := translatorForRequest(r)
	v := &wpView{}
	if e := s.listText(T, ctl.Request{Op: "wp.status", Site: row.Name}, v); e != "" {
		v.Error = wpMessageText(T, e)
		return v
	}
	if k, q := r.URL.Query().Get("k"), strings.TrimSpace(r.URL.Query().Get("q")); k != "" || q != "" {
		v.Kind, v.Term = k, q
		if _, err := apps.SearchArgs(k, q); err != nil {
			v.SearchErr = T("wp.err.term")
			return v
		}
		if e := s.listText(T, ctl.Request{Op: "wp.search", Site: row.Name, WPKind: k, WPTerm: q}, &v.Found); e != "" {
			v.SearchErr = wpMessageText(T, e)
		}
		// wordpress.org sends its texts already HTML-encoded (&quot;); the template encodes them once more, so they are decoded here.
		for i := range v.Found {
			v.Found[i].Name, v.Found[i].Short = html.UnescapeString(v.Found[i].Name), html.UnescapeString(v.Found[i].Short)
		}
	}
	return v
}

// wpRun does one thing to a plugin, a theme or WordPress itself, and shows the result on the WordPress tab.
func (s *Server) wpRun(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	row, status, msg := s.findSiteText(T, r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "wordpress")
	if row.App != apps.WordPressID {
		s.refuseTo(w, ss, back, T("wp.fail"), T("wp.err.notwp"))
		return
	}
	kind, action, slug := r.PostFormValue("kind"), r.PostFormValue("action"), strings.TrimSpace(r.PostFormValue("slug"))
	if _, err := apps.ManageArgs(kind, action, slug); err != nil {
		s.refuseTo(w, ss, back, T("wp.fail"), T("wp.err.action"))
		return
	}
	res := s.runText(T, T("act.wp."+action, slugOrCore(kind, slug), row.Name), ctl.Request{Op: "wp.run", Site: row.Name, WPKind: kind, WPAction: action, WPSlug: slug})
	res.Back = back
	if !res.OK {
		res.Error = wpMessageText(T, res.Error)
	}
	s.finish(w, r, res)
}

func slugOrCore(kind, slug string) string {
	if kind == "core" {
		return "WordPress"
	}
	return slug
}

// searchURL is the address of the WordPress tab with a search.
func searchURL(site, kind, term string) string {
	return siteTab(site, "wordpress") + "?" + url.Values{"k": {kind}, "q": {term}}.Encode()
}
