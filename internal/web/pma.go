package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/dbadmin"
	"mpn/internal/webauth"
)

// The database browser (phpMyAdmin) runs on a port of its own, so that a flaw in it could never read a page of the
// panel: two ports are two origins, and the panel's pages (with the form token in them) are out of reach. It is not
// open to anybody: nginx asks the panel, for every request, whether the visitor has a signed-in panel session.
//
// Opening it for a database goes like this. The administrator presses the button; the panel asks the root helper for a
// temporary database user (see dbadmin/temp.go) and keeps what it got under a ticket that can be used once, for one
// minute. The browser is sent to the browser's own sign-on page with the ticket in the address. That page, which runs on
// this machine, trades the ticket for the user and password over the loopback address, proving itself with a key that
// only it and the panel can read, and puts them in its session. The password never reaches the administrator's browser
// and is never stored by the panel.

const (
	pmaTicketTTL = time.Minute
	pmaMaxTicket = 200
)

type pmaTicket struct {
	User, Password, Database string
	Expires                  time.Time
}

type pmaTickets struct {
	mu sync.Mutex
	m  map[string]pmaTicket
}

func (t *pmaTickets) put(id string, v pmaTicket, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]pmaTicket{}
	}
	for k, x := range t.m {
		if now.After(x.Expires) {
			delete(t.m, k)
		}
	}
	if len(t.m) >= pmaMaxTicket {
		return false
	}
	t.m[id] = v
	return true
}

// take returns a ticket and removes it, so that it works once.
func (t *pmaTickets) take(id string, now time.Time) (pmaTicket, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.m[id]
	delete(t.m, id)
	if !ok || now.After(v.Expires) {
		return pmaTicket{}, false
	}
	return v, true
}

func (s *Server) pmaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /databases/pma", s.auth(s.pmaOpen))
	// These three are for the machine itself: nginx's sign-in check, and the sign-on page of the browser. The public nginx
	// server answers 404 for this whole path.
	mux.HandleFunc("GET /_pma/auth", s.pmaAuth)
	mux.HandleFunc("GET /_pma/ticket", s.pmaTrade)
	mux.HandleFunc("GET /_pma/end", s.pmaEnd)
}

func (s *Server) pmaOn() bool { return s.PMAPort > 0 && len(s.PMAKey) >= 32 }

func newTicketID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) pmaOpen(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	if !s.pmaOn() {
		http.NotFound(w, r)
		return
	}
	name := r.PostFormValue("name")
	fail := func(msg string) {
		s.show(w, http.StatusBadGateway, ss, view{Page: "result", Result: &result{Title: T("pma.title"), Error: msg, Back: "/"}})
	}
	if !dbadmin.ValidName(name) {
		s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: T("pma.title"), Error: T("err.baddb"), Back: "/"}})
		return
	}
	resp, err := s.ask(ctl.Request{Op: "db.pma", Name: name})
	switch {
	case err != nil:
		fail(T("err.helper", err.Error()))
		return
	case !resp.OK:
		fail(pmaFailure(resp))
		return
	}
	var visit struct {
		User, Password, Database string
		Expires                  int64
	}
	if json.Unmarshal([]byte(resp.Output), &visit) != nil || !dbadmin.ValidTempName(visit.User) {
		fail(T("err.badanswer"))
		return
	}
	id, err := newTicketID()
	now := time.Now()
	if err != nil || !s.tickets.put(id, pmaTicket{visit.User, visit.Password, visit.Database, now.Add(pmaTicketTTL)}, now) {
		_, _ = s.ask(ctl.Request{Op: "db.pmadrop", User: visit.User}) // nothing will use it
		fail(T("pma.busy"))
		return
	}
	target := url.URL{Scheme: "https", Host: hostOnly(r.Host) + ":" + strconv.Itoa(s.PMAPort), Path: "/signon.php", RawQuery: url.Values{"t": {id}}.Encode()}
	// A page with a link, not a redirect: the panel's policy lets a form post only to the panel, and a browser applies that
	// to where the post is sent on to. A link that the page's script follows is not a form post.
	s.show(w, 200, ss, view{Page: "pmaopen", PMALink: target.String(), PMAName: name})
}

// pmaFailure says in words why the helper could not make a temporary user. The helper's own text is for the command line
// ("exit status 2" and a line in English), so the cases a person can act on are told in Vietnamese.
func pmaFailure(resp ctl.Response) string {
	detail := strings.TrimSpace(resp.Output)
	switch {
	case strings.Contains(detail, "not found"):
		return T("pma.notfound")
	case strings.Contains(detail, "no database named"):
		return T("pma.gone")
	case detail != "":
		return T("pma.failed", detail)
	}
	return T("pma.failed", resp.Error)
}

// pmaAuth answers nginx's question "is this visitor signed in to the panel?" with 204 or 401.
func (s *Server) pmaAuth(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(s.cookieName()); err == nil {
		if _, ok := s.Sessions.Get(c.Value); ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	w.WriteHeader(http.StatusUnauthorized)
}

// keyOK checks the key that the sign-on page of the browser sends. A request without it is told nothing.
func (s *Server) keyOK(r *http.Request) bool {
	return s.pmaOn() && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Mpn-Key")), s.PMAKey) == 1
}

func (s *Server) pmaTrade(w http.ResponseWriter, r *http.Request) {
	if !s.keyOK(r) {
		http.NotFound(w, r)
		return
	}
	v, ok := s.tickets.take(r.URL.Query().Get("t"), time.Now())
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"user": v.User, "password": v.Password, "database": v.Database})
}

// pmaEnd drops the temporary user when the administrator signs out of the browser.
func (s *Server) pmaEnd(w http.ResponseWriter, r *http.Request) {
	u := r.URL.Query().Get("u")
	if !s.keyOK(r) || !dbadmin.ValidTempName(u) {
		http.NotFound(w, r)
		return
	}
	_, _ = s.ask(ctl.Request{Op: "db.pmadrop", User: u})
	w.WriteHeader(http.StatusNoContent)
}
