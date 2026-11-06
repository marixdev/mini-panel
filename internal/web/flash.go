package web

import (
	"crypto/sha256"
	"fmt"
	"net/http"

	"mpn/internal/webauth"
)

// The result of an operation is not sent back as the answer to the form post. It is kept for the session that did it,
// and the browser is sent to GET /result to read it once. Reloading that page then repeats nothing, because the form is
// not posted again, and a password shown there cannot be shown a second time.

func flashKey(token string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(token))) }

func (s *Server) putFlash(token string, r *result) {
	s.flashMu.Lock()
	defer s.flashMu.Unlock()
	if s.flash == nil || len(s.flash) > 100 { // bounded: results of sessions that never came back are dropped
		s.flash = map[string]*result{}
	}
	s.flash[flashKey(token)] = r
}

func (s *Server) takeFlash(token string) *result {
	s.flashMu.Lock()
	defer s.flashMu.Unlock()
	k := flashKey(token)
	r := s.flash[k]
	delete(s.flash, k)
	return r
}

// finish stores the result for this session and sends the browser to read it.
func (s *Server) finish(w http.ResponseWriter, r *http.Request, res *result) {
	if c, err := r.Cookie(s.cookieName()); err == nil {
		s.putFlash(c.Value, res)
	}
	http.Redirect(w, r, "/result", http.StatusSeeOther)
}

func (s *Server) resultPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	var res *result
	if c, err := r.Cookie(s.cookieName()); err == nil {
		res = s.takeFlash(c.Value)
	}
	if res == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.show(w, 200, ss, view{Page: "result", Result: res})
}
