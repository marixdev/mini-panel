package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"mpn/internal/ctl"
	"mpn/internal/dbadmin"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// siteRow is one line of `mpn site list -json`.
type siteRow struct {
	Name, Kind, User string
	Hostnames        []string
	Detail           string
	TLS              bool
	Root             string
	WWW              string `json:"www"`
	AllowHTTP        bool   `json:"allow_http"`
	Speed            bool   `json:"speed"`
	CertExpiry       int64  `json:"cert_expiry"`
	CustomVhost      bool   `json:"custom_vhost"`
}

// Primary is the first host name: the one the site is known by.
func (r siteRow) Primary() string {
	if len(r.Hostnames) == 0 {
		return r.Name
	}
	return r.Hostnames[0]
}

// Others are the host names after the first.
func (r siteRow) Others() []string {
	if len(r.Hostnames) < 2 {
		return nil
	}
	return r.Hostnames[1:]
}

// URL is the address of the site on the Internet, for the button that opens it.
func (r siteRow) URL() string {
	scheme := "http"
	if r.TLS {
		scheme = "https"
	}
	return scheme + "://" + r.Primary() + "/"
}

// Runtime names what the site runs on in a few words: "PHP 8.4", "Node.js 22", "Python 3.12", or its kind.
func (r siteRow) Runtime() string {
	f := strings.Fields(r.Detail)
	switch r.Kind {
	case "php", "node", "python":
		if len(f) >= 2 {
			return T("kind."+r.Kind) + " " + f[1]
		}
	}
	return T("kind." + r.Kind)
}

// result is what a finished operation shows: the helper's own words, never a guess.
type result struct {
	Title, Output, Error string
	OK                   bool
	Back                 string // the page the operation was started from; the home page when empty
}

// confirm describes a page that asks the user to type a name before something is destroyed.
type check struct {
	Name, Value, Label string
	On                 bool
}

type confirm struct {
	Checks                                 []check // boxes the user can untick, such as what to restore
	Back                                   string  // where the cancel link goes; the site list when empty
	Title, Warning, Action, Expect, Button string
	Hidden                                 map[string]string
	Purge                                  bool
	NoType                                 bool     // nothing to type: a plain confirmation
	Lines                                  []string // what will be affected, one per line
}

func (s *Server) ask(req ctl.Request) (ctl.Response, error) {
	if s.Call == nil {
		return ctl.Response{}, errors.New(T("err.nohelper"))
	}
	return s.Call(req)
}

var exitStatusRe = regexp.MustCompile(`^exit status \d+$`)

// plainError replaces what only a programmer reads ("exit status 2") with a sentence that sends the person to the details,
// which the page shows right below. Any other error message is kept as it is.
func plainError(e string) string {
	if exitStatusRe.MatchString(strings.TrimSpace(e)) {
		return T("err.exitstatus")
	}
	return e
}

// run sends the requests one after another and stops at the first that fails. The output of all of them is shown.
func (s *Server) run(title string, reqs ...ctl.Request) *result {
	res := &result{Title: title, OK: true}
	var out []string
	for _, rq := range reqs {
		resp, err := s.ask(rq)
		if t := strings.TrimRight(resp.Output, "\n"); t != "" {
			out = append(out, t)
		}
		if err != nil {
			res.OK, res.Error = false, T("err.helper", err.Error())
			break
		}
		if !resp.OK {
			res.OK, res.Error = false, plainError(resp.Error)
			break
		}
	}
	res.Output = strings.Join(out, "\n\n")
	return res
}

func (s *Server) show(w http.ResponseWriter, status int, ss webauth.Session, v view) {
	v.User, v.CSRF = ss.User, ss.CSRF
	s.render(w, status, v)
}

func (s *Server) sitePages(mux *http.ServeMux) {
	mux.HandleFunc("GET /sites/new", s.auth(s.newSitePage))
	mux.HandleFunc("POST /sites/add", s.auth(s.addSite))
	mux.HandleFunc("POST /sites/apply", s.auth(s.applySite))
	mux.HandleFunc("GET /sites/remove", s.auth(s.removeConfirm))
	mux.HandleFunc("POST /sites/remove", s.auth(s.removeSite))
}

func (s *Server) sitesPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v := view{Page: "sites", PHPVersions: site.PHPVersions, NodeVersions: site.NodeVersions}
	resp, err := s.ask(ctl.Request{Op: "site.list"})
	switch {
	case err != nil:
		v.Error = T("err.helper", err.Error())
	case !resp.OK:
		v.Error = resp.Error
	default:
		if err := json.Unmarshal([]byte(resp.Output), &v.Sites); err != nil {
			v.Error = T("err.badanswer")
		}
	}
	s.show(w, 200, ss, v)
}

func splitHosts(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
}

var siteKinds = []string{"php", "node", "python", "static", "proxy"}

// installedPHP and installedNode are the versions the form may offer: the ones on this machine, or every known one when the
// server was not told where to look (a test, or a build that does not know the machine).
func (s *Server) installedPHP() []string {
	if s.PHPDir == "" {
		return site.PHPVersions
	}
	return site.InstalledPHP(s.PHPDir)
}

func (s *Server) installedNode() []string {
	if s.NodeRoot == "" {
		return site.NodeVersions
	}
	return site.InstalledNode(s.NodeRoot)
}

func (s *Server) newSitePage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v := view{Page: "sitenew", Kinds: siteKinds, Reserved: reservedList(), PHPVersions: s.installedPHP(), NodeVersions: s.installedNode()}
	if k := r.URL.Query().Get("kind"); k != "" {
		if !slices.Contains(siteKinds, k) {
			http.Error(w, T("err.kind"), http.StatusBadRequest)
			return
		}
		v.Kind = k
	}
	if v.Kind != "" {
		// Filled in by the server, so the form has a password even if the script does not run.
		v.Password, _ = dbadmin.NewPassword()
	}
	s.show(w, 200, ss, v)
}

// siteNameFrom makes a site name from the first host name: "blog.example.com" becomes "blog-example-com".
func siteNameFrom(host string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(host) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	n := b.String()
	if len(n) > 31 {
		n = n[:31]
	}
	return strings.Trim(n, "-")
}

// addSite creates the site and applies it, then lets its own account log in over SSH and SFTP. The account name comes
// from the domain unless the form changed it, and the password is random unless one was typed. The account and the
// password are shown once, on the result page, because this is the only time they are known.
func (s *Server) addSite(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	nothing := func(msg string) { s.refuseTo(w, ss, "/sites/new", T("err.nothing"), msg) }
	hosts := splitHosts(strings.ToLower(r.PostFormValue("hosts")))
	if len(hosts) == 0 {
		nothing(T("err.nohosts"))
		return
	}
	if len(hosts) > 1 {
		nothing(T("err.onedomain"))
		return
	}
	name := siteNameFrom(hosts[0])
	if name == "" {
		nothing(T("err.sitename"))
		return
	}
	user := strings.TrimSpace(r.PostFormValue("user"))
	if user == "" {
		user = siteUserFrom(hosts[0])
	}
	if site.IsReservedUser(user) {
		nothing(T("err.userreserved", user))
		return
	}
	if s.accountExists(user) {
		nothing(T("err.userexists", user))
		return
	}
	// A runtime that is not installed would only fail at the last step with a message about a missing program.
	switch r.PostFormValue("kind") {
	case "php":
		if v := r.PostFormValue("php"); !slices.Contains(s.installedPHP(), v) {
			nothing(T("err.phpmissing", v))
			return
		}
	case "node":
		if v := r.PostFormValue("node"); !slices.Contains(s.installedNode(), v) {
			nothing(T("err.nodemissing", v))
			return
		}
	}
	password := r.PostFormValue("password")
	if password == "" {
		var err error
		if password, err = dbadmin.NewPassword(); err != nil {
			nothing(err.Error())
			return
		}
	}
	if !dbadmin.ValidPassword(password) {
		nothing(T("err.password"))
		return
	}
	// A name or an account that another website already holds is refused here, before anything is created, so that no
	// half-made site is left behind.
	var rows []siteRow
	if s.list(ctl.Request{Op: "site.list"}, &rows) == "" {
		for _, row := range rows {
			switch {
			case row.Name == name:
				nothing(T("err.siteused", name))
				return
			case row.User == user:
				nothing(T("err.userused", user, row.Name))
				return
			case slices.Contains(row.Hostnames, hosts[0]):
				nothing(T("dm.err.used", hosts[0], row.Name))
				return
			}
		}
	}
	res := s.run(T("act.site.create", name),
		ctl.Request{Op: "site.add", Site: name, User: user, Kind: r.PostFormValue("kind"), Hosts: hosts, PHP: r.PostFormValue("php"),
			Node: r.PostFormValue("node"), Python: r.PostFormValue("python"), Entry: strings.TrimSpace(r.PostFormValue("entry")), Upstream: strings.TrimSpace(r.PostFormValue("upstream"))},
		ctl.Request{Op: "site.apply", Site: name},
		ctl.Request{Op: "site.access", Site: name, Enable: true, Password: password})
	if res.OK {
		res.Output += "\n\n" + T("res.access", hostOnly(r.Host), user, password)
		res.Back = siteTab(name, "") // to the page of the website that was just made
	} else {
		res.Back = "/sites/new"
	}
	s.finish(w, r, res)
}

func (s *Server) applySite(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	name := r.PostFormValue("site")
	res := s.run(T("act.site.apply", name), ctl.Request{Op: "site.apply", Site: name})
	res.Back = safeBack(r.PostFormValue("back"), "/")
	s.finish(w, r, res)
}

func (s *Server) removeConfirm(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	name := r.URL.Query().Get("site")
	if !site.ValidName(name) {
		http.Error(w, T("err.badsite"), http.StatusBadRequest)
		return
	}
	s.show(w, 200, ss, view{Page: "confirm", Confirm: &confirm{
		Title: T("site.rm.title", name), Action: "/sites/remove", Expect: name, Button: T("site.rm.btn"), Purge: true, Back: "/",
		Warning: T("site.rm.warn"),
		Hidden:  map[string]string{"site": name}}})
}

func (s *Server) removeSite(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	name := r.PostFormValue("site")
	if !site.ValidName(name) || r.PostFormValue("confirm") != name {
		s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: T("site.rm.fail"), Error: T("err.confirm")}})
		return
	}
	req := ctl.Request{Op: "site.remove", Site: name, Purge: r.PostFormValue("purge") != ""}
	s.finish(w, r, s.run(T("act.site.remove", name), req))
}
