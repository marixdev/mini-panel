package web

import (
	"crypto/subtle"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"mpn/internal/webauth"
)

// The first account. Until it exists anybody who reaches the panel's port could make it, so the setup page asks for the key of the
// setup link: install.sh prints that link, and "mpn web setup-link" prints it again. The person opens the link once; the key goes
// into a short cookie and the address loses it, and the page then works as before. The account is made only where no file is, so
// the page can never replace an account that exists.

// setupCookie carries the key of the setup link from the first visit to the form.
const setupCookie = "mpn_setup"

// setupMu makes the check that no account exists and the writing of the first one a single step.
var setupMu sync.Mutex

// firstRun reports whether the panel has not been set up yet. Only a missing account file counts: an account file that cannot be
// read or is damaged is a problem for root to repair (mpn web passwd), never a reason to let a visitor make a new account.
func (s *Server) firstRun() bool {
	if s.Account == nil {
		return false
	}
	_, err := s.Account()
	return err != nil && errors.Is(err, fs.ErrNotExist)
}

var setupKeyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// setupKey is the key of the setup link, read from the file root wrote when the panel was installed. "" when there is none: then
// the page is closed, and root makes the account with mpn web passwd.
func (s *Server) setupKey() string {
	if s.SetupKeyFile == "" {
		return ""
	}
	b, err := os.ReadFile(s.SetupKeyFile)
	if err != nil {
		return ""
	}
	k := strings.TrimSpace(string(b))
	if !setupKeyRe.MatchString(k) {
		return ""
	}
	return k
}

func keyMatches(have, want string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(have), []byte(want)) == 1
}

func (s *Server) setupPages(mux routeMux) {
	mux.HandleFunc("GET /setup", s.setupGet)
	mux.HandleFunc("POST /setup", s.setupPost)
}

func (s *Server) setupCookieOf(r *http.Request) string {
	if c, err := r.Cookie(setupCookie); err == nil {
		return c.Value
	}
	return ""
}

func (s *Server) setupGet(w http.ResponseWriter, r *http.Request) {
	if !s.firstRun() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	want := s.setupKey()
	if t := r.URL.Query().Get("t"); t != "" {
		if !keyMatches(t, want) {
			s.render(w, http.StatusForbidden, view{Page: "setup", SetupLocked: true})
			return
		}
		// The key leaves the address at once, so it stays in no history and no Referer.
		http.SetCookie(w, &http.Cookie{Name: setupCookie, Value: t, Path: "/setup", MaxAge: 3600, HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if !keyMatches(s.setupCookieOf(r), want) {
		s.render(w, http.StatusForbidden, view{Page: "setup", SetupLocked: true})
		return
	}
	s.render(w, http.StatusOK, view{Page: "setup"})
}

var validSetupUser = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// setupPost makes the first account. Wrong setup keys from one address are limited like wrong passwords.
func (s *Server) setupPost(w http.ResponseWriter, r *http.Request) {
	T := translatorForWriter(w)
	if !s.firstRun() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !sameOrigin(r) {
		http.Error(w, "cross-site request refused", http.StatusForbidden)
		return
	}
	key := "setup:" + s.clientKey(r)
	if d := s.Throttle.Wait(key); d > 0 {
		s.render(w, http.StatusTooManyRequests, view{Page: "setup", SetupLocked: true, Error: T("login.locked", int(d.Seconds())+1)})
		return
	}
	if !keyMatches(s.setupCookieOf(r), s.setupKey()) {
		s.Throttle.Fail(key)
		s.render(w, http.StatusForbidden, view{Page: "setup", SetupLocked: true})
		return
	}
	refuse := func(msg string) {
		s.render(w, http.StatusBadRequest, view{Page: "setup", Error: msg})
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
	create := s.CreateAccount
	if create == nil {
		refuse(T("setup.err.nosave"))
		return
	}
	setupMu.Lock()
	defer setupMu.Unlock()
	if !s.firstRun() { // somebody else was quicker
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := create(webauth.Account{User: user, Hash: hash}); err != nil {
		if errors.Is(err, fs.ErrExist) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		refuse(T("setup.err.write", err.Error()))
		return
	}
	if s.SetupKeyFile != "" {
		_ = os.Remove(s.SetupKeyFile) // the link has done its work
	}
	s.note(r, "account.setup", "", "user="+user, false)
	s.Throttle.Reset(key)
	http.SetCookie(w, &http.Cookie{Name: setupCookie, Value: "", Path: "/setup", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: s.Secure})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
