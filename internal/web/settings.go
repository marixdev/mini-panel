package web

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"mpn/internal/webauth"
)

// The settings of the administrator's account: the password, and the two-step sign-in with a code from an authenticator app. The
// account file belongs to the panel's own account, so a change is written here and needs no help from the root helper. A person
// who has lost the app turns the second step off from the machine with "mpn web 2fa-off".

const (
	loginPendingTTL = 5 * time.Minute // from the right password to the code
	setupTTL        = 10 * time.Minute
	maxPending      = 20 // sign-ins waiting for a code at once
	maxCodeTries    = 5  // wrong codes for one sign-in before it has to start again
)

type pendingLogin struct {
	user  string
	exp   time.Time
	tries int
}

type pendingSetup struct {
	secret string
	exp    time.Time
}

// twoFactor keeps what is waiting in memory: sign-ins that passed the password and wait for a code, and secrets shown for a
// person to add to their app. A restart of the panel drops both, which only means starting that step again.
type twoFactor struct {
	mu    sync.Mutex
	login map[string]*pendingLogin
	setup map[string]*pendingSetup
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("no random numbers: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (t *twoFactor) newLogin(user string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if t.login == nil || len(t.login) >= maxPending {
		t.login = map[string]*pendingLogin{}
	}
	for k, p := range t.login {
		if now.After(p.exp) {
			delete(t.login, k)
		}
	}
	tok := newToken()
	t.login[flashKey(tok)] = &pendingLogin{user: user, exp: now.Add(loginPendingTTL)}
	return tok
}

// getLogin returns the sign-in a token names, if it is still waiting.
func (t *twoFactor) getLogin(tok string) (pendingLogin, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.login[flashKey(tok)]
	if !ok || time.Now().After(p.exp) {
		delete(t.login, flashKey(tok))
		return pendingLogin{}, false
	}
	return *p, true
}

// failLogin counts a wrong code and reports whether the sign-in may try again.
func (t *twoFactor) failLogin(tok string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.login[flashKey(tok)]
	if !ok {
		return false
	}
	if p.tries++; p.tries >= maxCodeTries {
		delete(t.login, flashKey(tok))
		return false
	}
	return true
}

func (t *twoFactor) doneLogin(tok string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.login, flashKey(tok))
}

func (t *twoFactor) putSetup(session, secret string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.setup == nil || len(t.setup) >= maxPending {
		t.setup = map[string]*pendingSetup{}
	}
	t.setup[flashKey(session)] = &pendingSetup{secret: secret, exp: time.Now().Add(setupTTL)}
}

func (t *twoFactor) getSetup(session string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.setup[flashKey(session)]
	if !ok || time.Now().After(p.exp) {
		delete(t.setup, flashKey(session))
		return "", false
	}
	return p.secret, true
}

func (t *twoFactor) dropSetup(session string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.setup, flashKey(session))
}

// settingsView is what the Settings page draws.
type settingsView struct {
	TOTPOn    bool
	Setup     bool          // the page shows the code to add to an app
	QR        template.HTML // the code, as an SVG
	Secret    string        // in groups of four, for typing
	SecretRaw string        // as the app wants it, for copying
	Error     string
	Which     string // the card the error belongs to: "password" or "2fa"
	Min       int
}

// qrSVG draws text as a QR code. It is an inline SVG because the page's policy allows no images from data: addresses.
func qrSVG(text, label string) (template.HTML, error) {
	q, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return "", err
	}
	bm := q.Bitmap() // includes the quiet border scanners need
	n := len(bm)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="192" height="192" class="qr" shape-rendering="crispEdges" role="img" aria-label="%s"><rect width="%d" height="%d" fill="#fff"/><path fill="#000" d="`,
		n, n, template.HTMLEscapeString(label), n, n)
	for y, row := range bm {
		for x := 0; x < n; {
			if !row[x] {
				x++
				continue
			}
			start := x
			for x < n && row[x] {
				x++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", start, y, x-start, x-start)
		}
	}
	b.WriteString(`"/></svg>`)
	return template.HTML(b.String()), nil
}

func (s *Server) settingsPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /settings", s.auth(s.settingsPage))
	mux.HandleFunc("GET /settings/2fa", s.auth(s.twoFASetupPage))
	mux.HandleFunc("POST /settings/password", s.auth(s.changePassword))
	mux.HandleFunc("POST /settings/2fa/enable", s.auth(s.twoFAEnable))
	mux.HandleFunc("POST /settings/2fa/disable", s.auth(s.twoFADisable))
	mux.HandleFunc("POST /login/2fa", s.loginSecondStep)
}

func (s *Server) sessionToken(r *http.Request) string {
	if c, err := r.Cookie(s.cookieName()); err == nil {
		return c.Value
	}
	return ""
}

// showSettings draws the page. The account is read again each time, so what the page says is what is stored.
func (s *Server) showSettings(w http.ResponseWriter, r *http.Request, ss webauth.Session, status int, v settingsView, notice *result) {
	v.Min = webauth.MinPasswordLen
	if acct, err := s.Account(); err == nil {
		v.TOTPOn = acct.TOTP != ""
	}
	s.show(w, status, ss, view{Page: "settings", Settings: &v, Notice: notice})
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.showSettings(w, r, ss, http.StatusOK, settingsView{}, s.takeFlash(s.sessionToken(r)))
}

// settingsDone remembers the result for this session and goes back to the page, which shows it once.
func (s *Server) settingsDone(w http.ResponseWriter, r *http.Request, title string) {
	s.putFlash(s.sessionToken(r), &result{Title: title, OK: true})
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

// guessKey is the key of the limit on guesses made from inside a signed-in session: somebody who got hold of a session must not be
// able to try passwords or codes without limit. A wrong try is logged like a wrong sign-in, so fail2ban sees it.
func (s *Server) guessKey(r *http.Request, what string) string { return what + ":" + s.clientKey(r) }

func (s *Server) tooMany(w http.ResponseWriter, r *http.Request, ss webauth.Session, key, which string) bool {
	T := translatorForWriter(w)
	if d := s.Throttle.Wait(key); d > 0 {
		w.Header().Set("Retry-After", fmt.Sprint(int(d.Seconds())+1))
		s.showSettings(w, r, ss, http.StatusTooManyRequests, settingsView{Error: T("login.locked", int(d.Seconds())+1), Which: which}, nil)
		return true
	}
	return false
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	fail := func(status int, msg string) {
		s.showSettings(w, r, ss, status, settingsView{Error: msg, Which: "password"}, nil)
	}
	key := s.guessKey(r, "pw")
	if s.tooMany(w, r, ss, key, "password") {
		return
	}
	acct, err := s.Account()
	if err != nil || s.SaveAccount == nil {
		fail(http.StatusServiceUnavailable, T("set.unavailable"))
		return
	}
	cur, pw, again := r.PostFormValue("current"), r.PostFormValue("new"), r.PostFormValue("again")
	if len(cur) > 256 || !acct.Check(acct.User, cur) {
		s.Throttle.Fail(key)
		log.Printf("login failed from %s", s.clientKey(r))
		fail(http.StatusUnauthorized, T("set.pw.wrong"))
		return
	}
	switch {
	case len(pw) < webauth.MinPasswordLen || len(pw) > 256:
		fail(http.StatusBadRequest, T("set.pw.short", webauth.MinPasswordLen))
		return
	case pw != again:
		fail(http.StatusBadRequest, T("set.pw.mismatch"))
		return
	case pw == cur:
		fail(http.StatusBadRequest, T("set.pw.same"))
		return
	}
	hash, err := webauth.Hash(pw)
	if err == nil {
		acct.Hash = hash
		err = s.SaveAccount(acct)
	}
	if err != nil {
		log.Printf("password could not be saved: %v", err)
		fail(http.StatusInternalServerError, T("set.save"))
		return
	}
	s.Throttle.Reset(key)
	s.Sessions.DeleteExcept(s.sessionToken(r)) // a browser signed in with the old password does not stay signed in
	s.settingsDone(w, r, T("set.pw.done"))
}

// twoFASetupPage makes a new secret and shows it as a QR code and as text. It is stored only in memory until a code from the app
// proves that the app has it.
func (s *Server) twoFASetupPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	acct, err := s.Account()
	if err != nil || s.SaveAccount == nil {
		s.showSettings(w, r, ss, http.StatusServiceUnavailable, settingsView{Error: T("set.unavailable"), Which: "2fa"}, nil)
		return
	}
	if acct.TOTP != "" {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	secret, err := webauth.NewTOTPSecret()
	if err != nil {
		http.Error(w, "no random numbers", http.StatusInternalServerError)
		return
	}
	s.twofa.putSetup(s.sessionToken(r), secret)
	s.showSetup(w, r, ss, http.StatusOK, acct.User, secret, "")
}

func (s *Server) showSetup(w http.ResponseWriter, r *http.Request, ss webauth.Session, status int, user, secret, problem string) {
	T := translatorForWriter(w)
	qr, err := qrSVG(webauth.TOTPURI(T("app.name"), user, secret), T("set.2fa.qr.alt"))
	if err != nil {
		log.Printf("qr code: %v", err)
	}
	s.showSettings(w, r, ss, status, settingsView{Setup: true, QR: qr, Secret: webauth.GroupSecret(secret), SecretRaw: secret, Error: problem, Which: "2fa"}, nil)
}

func (s *Server) twoFAEnable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	key := s.guessKey(r, "2fa")
	if s.tooMany(w, r, ss, key, "2fa") {
		return
	}
	acct, err := s.Account()
	secret, ok := s.twofa.getSetup(s.sessionToken(r))
	if err != nil || s.SaveAccount == nil || !ok {
		http.Redirect(w, r, "/settings/2fa", http.StatusSeeOther) // the secret is gone (a restart, or ten minutes): start again
		return
	}
	step, good := webauth.VerifyTOTP(secret, r.PostFormValue("code"), time.Now(), 0)
	if !good {
		s.Throttle.Fail(key)
		log.Printf("login failed from %s", s.clientKey(r))
		s.showSetup(w, r, ss, http.StatusUnauthorized, acct.User, secret, T("set.2fa.wrong"))
		return
	}
	acct.TOTP, acct.TOTPStep = secret, step
	if err := s.SaveAccount(acct); err != nil {
		log.Printf("two-step sign-in could not be saved: %v", err)
		s.showSetup(w, r, ss, http.StatusInternalServerError, acct.User, secret, T("set.save"))
		return
	}
	s.Throttle.Reset(key)
	s.twofa.dropSetup(s.sessionToken(r))
	s.settingsDone(w, r, T("set.2fa.done"))
}

// twoFADisable needs both the password and a current code: a session that somebody else got hold of cannot take the second step off.
func (s *Server) twoFADisable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	key := s.guessKey(r, "2fa")
	if s.tooMany(w, r, ss, key, "2fa") {
		return
	}
	acct, err := s.Account()
	if err != nil || s.SaveAccount == nil || acct.TOTP == "" {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	pw := r.PostFormValue("password")
	_, codeOK := webauth.VerifyTOTP(acct.TOTP, r.PostFormValue("code"), time.Now(), acct.TOTPStep)
	if len(pw) > 256 || !acct.Check(acct.User, pw) || !codeOK {
		s.Throttle.Fail(key)
		log.Printf("login failed from %s", s.clientKey(r))
		s.showSettings(w, r, ss, http.StatusUnauthorized, settingsView{Error: T("set.2fa.off.wrong"), Which: "2fa"}, nil)
		return
	}
	acct.TOTP, acct.TOTPStep = "", 0
	if err := s.SaveAccount(acct); err != nil {
		log.Printf("two-step sign-in could not be saved: %v", err)
		s.showSettings(w, r, ss, http.StatusInternalServerError, settingsView{Error: T("set.save"), Which: "2fa"}, nil)
		return
	}
	s.Throttle.Reset(key)
	s.settingsDone(w, r, T("set.2fa.off.done"))
}

// loginSecondStep is the code that follows a right password. The sign-in is a token held in memory; it ends after a few wrong
// codes or a few minutes, and a code that was used cannot be used again.
func (s *Server) loginSecondStep(w http.ResponseWriter, r *http.Request) {
	T := translatorForWriter(w)
	key := s.clientKey(r)
	tok, code := r.PostFormValue("pending"), r.PostFormValue("code")
	expired := func() {
		s.render(w, http.StatusUnauthorized, view{Page: "login", Error: T("login.2fa.expired")})
	}
	p, ok := s.twofa.getLogin(tok)
	if !ok {
		expired()
		return
	}
	if d := s.Throttle.Wait(key); d > 0 {
		w.Header().Set("Retry-After", fmt.Sprint(int(d.Seconds())+1))
		s.render(w, http.StatusTooManyRequests, view{Page: "login", Need2FA: true, Pending: tok, Error: T("login.locked", int(d.Seconds())+1)})
		return
	}
	acct, err := s.Account()
	if err != nil || acct.User != p.user || acct.TOTP == "" {
		s.twofa.doneLogin(tok)
		expired()
		return
	}
	step, good := webauth.VerifyTOTP(acct.TOTP, code, time.Now(), acct.TOTPStep)
	if !good {
		s.Throttle.Fail(key)
		log.Printf("login failed from %s", key) // read by the mpn-panel jail of fail2ban
		if !s.twofa.failLogin(tok) {
			expired()
			return
		}
		s.render(w, http.StatusUnauthorized, view{Page: "login", Need2FA: true, Pending: tok, Error: T("login.2fa.wrong")})
		return
	}
	acct.TOTPStep = step
	if s.SaveAccount != nil {
		if err := s.SaveAccount(acct); err != nil {
			log.Printf("the used code could not be remembered: %v", err)
		}
	}
	s.twofa.doneLogin(tok)
	s.Throttle.Reset(key)
	s.startSession(w, r, acct.User)
}
