package web

import (
	"net/http"
	"strings"

	"mpn/internal/apps"
	"mpn/internal/ctl"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// appOption is one application the form offers.
type appOption struct {
	ID, Label, MinPHP string
	DB                bool
}

func appOptions() []appOption {
	var out []appOption
	for _, r := range apps.All() {
		out = append(out, appOption{ID: r.ID, Label: r.Label, MinPHP: r.MinPHP, DB: r.NeedsDB()})
	}
	return out
}

func appLabel(id string) string {
	if r, ok := apps.Get(id); ok {
		return r.Label
	}
	return id
}

// appForm is what the person chose for the code of a PHP site.
type appForm struct{ App, Title, Admin, Email, Locale string }

func readAppForm(r *http.Request) appForm {
	f := appForm{App: r.PostFormValue("app"), Title: strings.TrimSpace(r.PostFormValue("wp_title")), Admin: strings.TrimSpace(r.PostFormValue("wp_admin")),
		Email: strings.TrimSpace(r.PostFormValue("wp_email")), Locale: r.PostFormValue("wp_locale")}
	if f.Locale == "" {
		f.Locale = "vi"
	}
	return f
}
func (f appForm) check(phpVersion string) string {
	return f.checkText(T, phpVersion)
}

// check says in words what is wrong with the choice, or returns "". It runs before anything is created, so a form that cannot work leaves
// nothing behind.
func (f appForm) checkText(T translator, phpVersion string) string {
	if f.App == "" {
		return ""
	}
	rec, ok := apps.Get(f.App)
	switch {
	case !ok:
		return T("app.err.unknown")
	case !apps.PHPAtLeast(phpVersion, rec.MinPHP):
		return T("app.err.php", rec.Label, rec.MinPHP, phpVersion)
	case rec.ID != apps.WordPressID:
		return ""
	case !apps.ValidTitle(f.Title):
		return T("app.err.title")
	case !apps.ValidAdminUser(f.Admin):
		return T("app.err.admin")
	case !apps.ValidEmail(f.Email):
		return T("app.err.email")
	case !apps.ValidLocale(f.Locale):
		return T("app.err.locale")
	}
	return ""
}

func (f appForm) request(siteName string) ctl.Request {
	return ctl.Request{Op: "app.install", Site: siteName, App: f.App, AppTitle: f.Title, AppAdmin: f.Admin, AppEmail: f.Email, AppLocale: f.Locale}
}
func appError(out string) string {
	return appErrorText(T, out)
}

// appError turns the refusals of the installation into a sentence for the person.
func appErrorText(T translator, out string) string {
	for _, m := range []struct{ marker, key string }{{"app-notempty", "app.err.notempty"}, {"app-php", "app.err.phpold"}, {"app-installed", "app.err.installed"},
		{"app-kind", "app.err.kind"}, {"app-db", "app.err.db"}, {"does not match its checksum", "app.err.sum"}} {
		if strings.Contains(out, m.marker) {
			return T(m.key)
		}
	}
	return ""
}

func (s *Server) appRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sites/{name}/app/install", s.auth(s.appInstall))
}

// appInstall puts an application in a PHP site that has none yet: the way to try again when the installation that follows creating a
// site failed, and to install one into a site that was made empty.
func (s *Server) appInstall(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	row, status, msg := s.findSiteText(T, r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "")
	f := readAppForm(r)
	switch {
	case row.Kind != string(site.PHP):
		s.refuseTo(w, ss, back, T("app.fail"), T("app.err.kind"))
		return
	case row.App != "":
		s.refuseTo(w, ss, back, T("app.fail"), T("app.err.installed"))
		return
	case f.App == "":
		s.refuseTo(w, ss, back, T("app.fail"), T("app.err.unknown"))
		return
	}
	if p := f.checkText(T, row.PHPVersion()); p != "" {
		s.refuseTo(w, ss, back, T("app.fail"), p)
		return
	}
	res := s.runText(T, T("act.app.install", appLabel(f.App), row.Name), f.request(row.Name))
	res.Back = back
	if !res.OK {
		if e := appErrorText(T, res.Output); e != "" {
			res.Error = e
		}
	}
	s.finish(w, r, res)
}
