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

func (s *Server) list(req ctl.Request, dst any) string {
	return s.listText(T, req, dst)
}

// list asks the helper for a JSON list and decodes it into dst. It returns the text of the problem, or "".
func (s *Server) listText(T translator, req ctl.Request, dst any) string {
	resp, err := s.askText(T, req)
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
	// The list of databases is the Databases tab of each website; only the actions have addresses of their own.
	mux.HandleFunc("POST /databases/add", s.auth(s.addDB))
	mux.HandleFunc("GET /databases/passwd", s.auth(s.dbConfirm("passwd")))
	mux.HandleFunc("POST /databases/passwd", s.auth(s.dbAct("passwd")))
	mux.HandleFunc("GET /databases/remove", s.auth(s.dbConfirm("remove")))
	mux.HandleFunc("POST /databases/remove", s.auth(s.dbAct("remove")))
	mux.HandleFunc("GET /databases/remote", s.auth(s.dbRemotePage))
	mux.HandleFunc("POST /databases/remote", s.auth(s.dbRemoteSave))
}

// dbRemote is what the page of remote access draws: the addresses a database can be reached from, in a box to edit.
type dbRemote struct {
	Name, Site, Hosts, Error, ServerIP string
	Listening, FirewallOn, Any         bool
	Port                               int
}

func (s *Server) dbRemoteView(T translator, r *http.Request, name, site string) (*dbRemote, string) {
	if !dbadmin.ValidName(name) {
		return nil, T("err.baddb")
	}
	var info struct {
		Hosts      []string `json:"hosts"`
		Listening  bool     `json:"listening"`
		FirewallOn bool     `json:"firewall_on"`
		Port       int      `json:"port"`
	}
	if e := s.listText(T, ctl.Request{Op: "db.remoteinfo", Name: name}, &info); e != "" {
		return nil, e
	}
	v := &dbRemote{Name: name, Site: site, Hosts: strings.Join(info.Hosts, "\n"), Listening: info.Listening, FirewallOn: info.FirewallOn, Port: info.Port, ServerIP: serverIP(r.Host)}
	for _, h := range info.Hosts {
		v.Any = v.Any || h == dbadmin.AnyHost
	}
	return v, ""
}

func (s *Server) dbRemotePage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	site := r.URL.Query().Get("site")
	v, e := s.dbRemoteView(T, r, r.URL.Query().Get("name"), site)
	if e != "" {
		s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: T("db.remote"), Error: e, Back: siteTab(site, "databases")}})
		return
	}
	s.show(w, http.StatusOK, ss, view{Page: "dbremote", DBRemote: v})
}

func (s *Server) dbRemoteSave(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	name, site := r.PostFormValue("name"), r.PostFormValue("site")
	back := siteTab(site, "databases")
	if !dbadmin.ValidName(name) {
		s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: T("db.remote"), Error: T("err.baddb"), Back: back}})
		return
	}
	hosts := []string{}
	if r.PostFormValue("close") != "1" {
		list, err := dbadmin.ParseHosts(r.PostFormValue("hosts"))
		if err != nil {
			v, _ := s.dbRemoteView(T, r, name, site)
			if v == nil {
				v = &dbRemote{Name: name, Site: site}
			}
			v.Hosts, v.Error = r.PostFormValue("hosts"), T("db.remote.err.hosts")
			s.show(w, http.StatusBadRequest, ss, view{Page: "dbremote", DBRemote: v})
			return
		}
		hosts = list
	}
	res := s.runText(r, T, T("act.db.remote", name), ctl.Request{Op: "db.remote", Name: name, Hosts: hosts})
	res.Back = back
	s.finish(w, r, res)
}

func (s *Server) addDB(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	site := r.PostFormValue("site")
	back := safeBack(r.PostFormValue("back"), siteTab(site, "databases"))
	name, problem := dbNameForText(T, site, r.PostFormValue("name"))
	if problem != "" {
		// Said here, in Vietnamese, before the helper is asked: its own message is for the command line.
		s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: T("act.db.add", site), Error: problem, Back: back}})
		return
	}
	res := s.runText(r, T, T("act.db.add", site), ctl.Request{Op: "db.add", Site: site, Name: name})
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
func dbNameFor(site, typed string) (name,
	problem string) {
	return dbNameForText(T, site, typed)
}

// dbNameFor turns what was typed into the name sent to the helper. Nothing typed means the helper picks the default
// (the site name with "-" as "_"). A name that already starts the right way is kept; anything else is taken as the end of
// the name, so typing "shop" for the site "blog" gives "blog_shop". Capitals become small letters and "-" becomes "_",
// because the person is not expected to know the rules of the database server.
func dbNameForText(T translator, site, typed string) (name, problem string) {
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
		T := translatorForWriter(w)
		name := r.URL.Query().Get("name")
		if !dbadmin.ValidName(name) {
			http.Error(w, T("err.baddb"), http.StatusBadRequest)
			return
		}
		back := safeBack(r.URL.Query().Get("back"), "/")
		s.show(w, 200, ss, view{Page: "confirm", Confirm: &confirm{Title: T(a.title, name), Warning: T(a.warning), Back: back, Action: "/databases/" + action,
			Expect: name, Button: T(a.button), Hidden: map[string]string{"name": name, "back": back}}})
	}
}

func (s *Server) dbAct(action string) func(http.ResponseWriter, *http.Request, webauth.Session) {
	a := dbActions[action]
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		T := translatorForWriter(w)
		name := r.PostFormValue("name")
		back := safeBack(r.PostFormValue("back"), "/")
		if !dbadmin.ValidName(name) || r.PostFormValue("confirm") != name {
			s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: T("err.nothing"), Error: T("err.confirm"), Back: back}})
			return
		}
		res := s.runText(r, T, T(a.title, name), ctl.Request{Op: "db." + action, Name: name})
		res.Back = back
		s.finish(w, r, res)
	}
}
