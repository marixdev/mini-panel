package web

import (
	"net/http"
	"regexp"
	"strings"

	"mpn/internal/webauth"
)

// firstRun reports whether the panel has not been set up yet: the account file is missing or unreadable.
func (s *Server) firstRun() bool {
	if s.Account == nil {
		return false
	}
	_, err := s.Account()
	return err != nil
}

func (s *Server) setupPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /setup", s.setupGet)
	mux.HandleFunc("POST /setup", s.setupPost)
}

func (s *Server) setupGet(w http.ResponseWriter, r *http.Request) {
	if !s.firstRun() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, http.StatusOK, view{Page: "setup"})
}

var validSetupUser = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func (s *Server) setupPost(w http.ResponseWriter, r *http.Request) {
	T := translatorForWriter(w)
	if !s.firstRun() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	refuse := func(msg string) {
		s.render(w, http.StatusBadRequest, view{Page: "setup", Error: msg})
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
		return
	}
	user := strings.TrimSpace(r.PostFormValue("user"))
	if !validSetupUser.MatchString(user) {
		refuse(T("setup.err.user"))
		return
	}
	pw := r.PostFormValue("password")
	pw2 := r.PostFormValue("password2")
	if len(pw) < webauth.MinPasswordLen {
		refuse(T("setup.err.short", webauth.MinPasswordLen))
		return
	}
	if len(pw) > 256 {
		refuse(T("setup.err.long"))
		return
	}
	if pw != pw2 {
		refuse(T("setup.err.match"))
		return
	}
	hash, err := webauth.Hash(pw)
	if err != nil {
		refuse(T("setup.err.hash"))
		return
	}
	if s.SaveAccount == nil {
		refuse(T("setup.err.nosave"))
		return
	}
	if err := s.SaveAccount(webauth.Account{User: user, Hash: hash}); err != nil {
		refuse(T("setup.err.write", err.Error()))
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
