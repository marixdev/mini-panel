package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/dbadmin"
	"mpn/internal/webauth"
)

// dbRow is one line of `mpn db list -json`.
type dbRow struct{ Name, Site string }

// list asks the helper for a JSON list and decodes it into dst. It returns the text of the problem, or "".
func (s *Server) list(req ctl.Request, dst any) string {
	resp, err := s.ask(req)
	switch {
	case err != nil:
		return T("err.helper", err.Error())
	case !resp.OK:
		return resp.Error
	}
	if json.Unmarshal([]byte(resp.Output), dst) != nil {
		return T("err.badanswer")
	}
	return ""
}

func (s *Server) dbPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /databases", s.auth(s.databasesPage))
	mux.HandleFunc("POST /databases/add", s.auth(s.addDB))
	mux.HandleFunc("GET /databases/passwd", s.auth(s.dbConfirm("passwd")))
	mux.HandleFunc("POST /databases/passwd", s.auth(s.dbAct("passwd")))
	mux.HandleFunc("GET /databases/remove", s.auth(s.dbConfirm("remove")))
	mux.HandleFunc("POST /databases/remove", s.auth(s.dbAct("remove")))
}

func (s *Server) databasesPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v := view{Page: "databases"}
	var sites []siteRow
	v.PMA = s.pmaOn()
	v.Error = s.list(ctl.Request{Op: "db.list"}, &v.DBs)
	if e := s.list(ctl.Request{Op: "site.list"}, &sites); e != "" && v.Error == "" {
		v.Error = e
	}
	for _, x := range sites {
		v.SiteNames = append(v.SiteNames, x.Name)
	}
	s.show(w, 200, ss, v)
}

func (s *Server) addDB(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	site := r.PostFormValue("site")
	back := safeBack(r.PostFormValue("back"), "/databases")
	name, problem := dbNameFor(site, r.PostFormValue("name"))
	if problem != "" {
		// Said here, in Vietnamese, before the helper is asked: its own message is for the command line.
		s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: T("act.db.add", site), Error: problem, Back: back}})
		return
	}
	res := s.run(T("act.db.add", site), ctl.Request{Op: "db.add", Site: site, Name: name})
	res.Back = back
	if !res.OK && (strings.Contains(res.Output, "already taken") || strings.Contains(res.Output, "already exists on the server")) {
		shown := name
		if shown == "" {
			shown = dbadmin.BaseName(site)
		}
		res.Error = T("db.err.taken", shown)
	}
	s.finish(w, r, res)
}

// dbNameFor turns what was typed into the name sent to the helper. Nothing typed means the helper picks the default
// (the site name with "-" as "_"). A name that already starts the right way is kept; anything else is taken as the end of
// the name, so typing "shop" for the site "blog" gives "blog_shop". Capitals become small letters and "-" becomes "_",
// because the person is not expected to know the rules of the database server.
func dbNameFor(site, typed string) (name, problem string) {
	typed = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(typed)), "-", "_")
	if typed == "" {
		return "", ""
	}
	for _, c := range typed {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return "", T("db.err.chars")
		}
	}
	base := dbadmin.BaseName(site)
	if !dbadmin.OwnedBy(typed, site) {
		typed = base + "_" + typed
	}
	if !dbadmin.ValidName(typed) {
		if room := 32 - len(base) - 1; room > 0 {
			return "", T("db.err.long", base, room)
		}
		return "", T("db.err.sitelong", base)
	}
	return typed, ""
}

var dbActions = map[string]struct{ title, warning, button string }{
	"passwd": {"act.db.passwd", "db.pw.warn", "db.pw.btn"},
	"remove": {"act.db.remove", "db.rm.warn", "db.rm.btn"},
}

func (s *Server) dbConfirm(action string) func(http.ResponseWriter, *http.Request, webauth.Session) {
	a := dbActions[action]
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		name := r.URL.Query().Get("name")
		if !dbadmin.ValidName(name) {
			http.Error(w, T("err.baddb"), http.StatusBadRequest)
			return
		}
		back := safeBack(r.URL.Query().Get("back"), "/databases")
		s.show(w, 200, ss, view{Page: "confirm", Confirm: &confirm{Title: T(a.title, name), Warning: T(a.warning), Back: back, Action: "/databases/" + action,
			Expect: name, Button: T(a.button), Hidden: map[string]string{"name": name, "back": back}}})
	}
}

func (s *Server) dbAct(action string) func(http.ResponseWriter, *http.Request, webauth.Session) {
	a := dbActions[action]
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		name := r.PostFormValue("name")
		back := safeBack(r.PostFormValue("back"), "/databases")
		if !dbadmin.ValidName(name) || r.PostFormValue("confirm") != name {
			s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: T("err.nothing"), Error: T("err.confirm"), Back: back}})
			return
		}
		res := s.run(T(a.title, name), ctl.Request{Op: "db." + action, Name: name})
		res.Back = back
		s.finish(w, r, res)
	}
}
