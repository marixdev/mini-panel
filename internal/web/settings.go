package web

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/qr"
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
	user     string
	remember bool // the person ticked "remember this browser" with the password
	exp      time.Time
	tries    int
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

func (t *twoFactor) newLogin(user string, remember bool) string {
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
	t.login[flashKey(tok)] = &pendingLogin{user: user, remember: remember, exp: now.Add(loginPendingTTL)}
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
	Version   string // of the program that runs
	UpdateLog string // the last update, as mpn update wrote it
	Sessions  int    // browsers signed in now, this one included
	Delegated bool   // a delegated user is signed in: the page shows its own account only
}

// updateLog is what the last "mpn update" wrote; the root helper makes the file and gives it to mpn-web.
var updateLog = "/var/lib/mpn-web/update.log"

func readUpdateLog() string {
	b, err := os.ReadFile(updateLog)
	if err != nil {
		return ""
	}
	if len(b) > 8<<10 {
		b = b[len(b)-8<<10:]
	}
	return strings.TrimSpace(string(b))
}

// updateCheck asks GitHub now, from the "Check now" button. The panel looks by itself every few hours; this is for somebody who knows
// a release just came out. Without the checker (a test, an old setup) it asks the helper as before.
func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	if s.Updates != nil {
		s.updateCheckNow(w, r)
		return
	}
	T := translatorForWriter(w)
	res := s.runText(r, T, T("set.up.check.title"), ctl.Request{Op: "panel.updatecheck"})
	res.Back = "/update"
	s.finish(w, r, res)
}

// updateStart starts "mpn update" in the background. The helper checks the version again, and mpn update checks the signature of
// the release against the key built into the program; nothing the page sends can choose a key or a place to download from.
func (s *Server) updateStart(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	v := strings.TrimSpace(r.PostFormValue("version"))
	if v != "" && !versionText.MatchString(v) {
		s.refuseTo(w, ss, "/update", T("err.nothing"), T("set.up.err.version"))
		return
	}
	res := s.runText(r, T, T("set.up.start.title"), ctl.Request{Op: "panel.update", Version: v})
	res.Back = "/update"
	s.finish(w, r, res)
}

// updatePageView is what the Update page shows about releases: the newer one with the changelog of everything after this version, or
// that this is the latest, and when GitHub was last asked.
type updatePageView struct {
	Latest    string // the newer release, "" when this is the latest or nothing is known
	Known     string // the latest release GitHub named, even when it is this one
	Changelog []changelogEntry
	Checked   time.Time
	CheckErr  string
	Auto      bool // the panel looks by itself
}

type changelogEntry struct {
	Version, Date string
	Body          template.HTML
}

// updatePage shows the version that runs, a newer release with its changelog, the button to update, and what the last update wrote.
func (s *Server) updatePage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v := settingsView{Version: Version, UpdateLog: readUpdateLog()}
	up := &updatePageView{}
	if s.Updates != nil {
		info := s.Updates.Info()
		latest, sections := s.Updates.Available(writerLanguage(w))
		up.Auto, up.Latest, up.Known, up.Checked, up.CheckErr = true, latest, info.Latest, info.Checked, info.Error
		for _, sec := range sections {
			up.Changelog = append(up.Changelog, changelogEntry{Version: sec.Version, Date: sec.Date, Body: changelogHTML(sec.Body)})
		}
	}
	s.show(w, http.StatusOK, ss, view{Page: "update", Settings: &v, Up: up, Notice: s.takeFlash(s.sessionToken(r))})
}

var versionText = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}$`)

// qrSVG draws text as a QR code. It is an inline SVG because the page's policy allows no images from data: addresses.
func qrSVG(text, label string) (template.HTML, error) {
	sym, err := qr.Encode(text)
	if err != nil {
		return "", err
	}
	// The light border of four modules that scanners need.
	const quiet = 4
	n := len(sym) + 2*quiet
	bm := make([][]bool, n)
	for y := range bm {
		bm[y] = make([]bool, n)
		if y >= quiet && y < n-quiet {
			copy(bm[y][quiet:], sym[y-quiet])
		}
	}
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

func (s *Server) settingsPages(mux routeMux) {
	mux.HandleFunc("GET /settings", s.auth(s.settingsPage))
	mux.HandleFunc("GET /settings/2fa", s.auth(s.twoFASetupPage))
	mux.HandleFunc("POST /settings/password", s.auth(s.changePassword))
	mux.HandleFunc("POST /settings/2fa/enable", s.auth(s.twoFAEnable))
	mux.HandleFunc("POST /settings/2fa/disable", s.auth(s.twoFADisable))
	mux.HandleFunc("POST /settings/sessions/end", s.auth(s.endOtherSessions))
	mux.HandleFunc("GET /update", s.auth(s.updatePage))
	mux.HandleFunc("POST /update/check", s.auth(s.updateCheck))
	mux.HandleFunc("POST /update", s.auth(s.updateStart))
	// The addresses of 1.2.0, kept so that a form left open in a tab still works.
	mux.HandleFunc("POST /settings/update/check", s.auth(s.updateCheck))
	mux.HandleFunc("POST /settings/update", s.auth(s.updateStart))
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
	v.Sessions = s.Sessions.Count()
	if sa, err := s.loadSettingsAccount(ss); err == nil {
		v.TOTPOn = sa.acct.TOTP != ""
		v.Delegated = sa.delegated
		if sa.delegated {
			v.Sessions = s.Sessions.CountUser(ss.User)
		}
	}
	s.show(w, status, ss, view{Page: "settings", Settings: &v, Notice: notice})
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.showSettings(w, r, ss, http.StatusOK, settingsView{}, s.takeFlash(s.sessionToken(r)))
}

// settingsAccount is the account a Settings action works on: the administrator's account file, or one delegated user's record in the
// users file. The delegated flag says which, and acct is the password side of it.
type settingsAccount struct {
	acct      webauth.Account
	user      webauth.User // the delegated record, when delegated
	delegated bool
}

// loadSettingsAccount reads the account of the signed-in session. A session that is not the administrator's is a delegated user.
func (s *Server) loadSettingsAccount(ss webauth.Session) (settingsAccount, error) {
	if s.isAdmin(ss) {
		acct, err := s.Account()
		return settingsAccount{acct: acct}, err
	}
	rec, ok := s.userRecord(ss.User)
	if !ok {
		return settingsAccount{}, errors.New("no such delegated user")
	}
	return settingsAccount{acct: rec.Account(), user: rec, delegated: true}, nil
}

// saveSettingsAccount writes the account back: the administrator's file, or the delegated user's record in the users file.
func (s *Server) saveSettingsAccount(sa settingsAccount) error {
	if !sa.delegated {
		return s.SaveAccount(sa.acct)
	}
	u := sa.user
	u.Hash, u.TOTP, u.TOTPStep = sa.acct.Hash, sa.acct.TOTP, sa.acct.TOTPStep
	return s.saveDelegated(u)
}

// canSave reports whether the Settings action can write the account at all.
func (s *Server) canSave(sa settingsAccount) bool {
	return sa.delegated || s.SaveAccount != nil
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
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	sa, err := s.loadSettingsAccount(ss)
	acct := sa.acct
	if err != nil || !s.canSave(sa) {
		fail(http.StatusServiceUnavailable, T("set.unavailable"))
		return
	}
	cur, pw, again := r.PostFormValue("current"), r.PostFormValue("new"), r.PostFormValue("again")
	if len(cur) > 256 || !acct.Check(acct.User, cur) {
		s.Throttle.Fail(key)
		s.note(r, "account.password_fail", "", "user="+ss.User, true)
		log.Printf("login failed from %s", s.clientAddr(r))
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
		sa.acct = acct
		err = s.saveSettingsAccount(sa)
	}
	if err != nil {
		log.Printf("password could not be saved: %v", err)
		fail(http.StatusInternalServerError, T("set.save"))
		return
	}
	s.Throttle.Reset(key)
	s.note(r, "account.password", "", "user="+acct.User, false)
	if sa.delegated {
		s.Sessions.DeleteUserExcept(ss.User, s.sessionToken(r))
	} else {
		s.Sessions.DeleteExcept(s.sessionToken(r)) // a browser signed in with the old password does not stay signed in
	}
	s.settingsDone(w, r, T("set.pw.done"))
}

// endOtherSessions signs out every browser but this one, remembered ones too: for a lost laptop or a shared computer left signed in.
func (s *Server) endOtherSessions(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	if sa, err := s.loadSettingsAccount(ss); err == nil && sa.delegated {
		s.Sessions.DeleteUserExcept(ss.User, s.sessionToken(r))
	} else {
		s.Sessions.DeleteExcept(s.sessionToken(r))
	}
	s.note(r, "account.sessions_end", "", "user="+ss.User, false)
	s.settingsDone(w, r, T("set.sess.done"))
}

// twoFASetupPage makes a new secret and shows it as a QR code and as text. It is stored only in memory until a code from the app
// proves that the app has it.
func (s *Server) twoFASetupPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	sa, err := s.loadSettingsAccount(ss)
	acct := sa.acct
	if err != nil || !s.canSave(sa) {
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
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	sa, err := s.loadSettingsAccount(ss)
	acct := sa.acct
	secret, ok := s.twofa.getSetup(s.sessionToken(r))
	if err != nil || !s.canSave(sa) || !ok {
		http.Redirect(w, r, "/settings/2fa", http.StatusSeeOther) // the secret is gone (a restart, or ten minutes): start again
		return
	}
	step, good := webauth.VerifyTOTP(secret, r.PostFormValue("code"), time.Now(), 0)
	if !good {
		s.Throttle.Fail(key)
		log.Printf("login failed from %s", s.clientAddr(r))
		s.showSetup(w, r, ss, http.StatusUnauthorized, acct.User, secret, T("set.2fa.wrong"))
		return
	}
	acct.TOTP, acct.TOTPStep = secret, step
	sa.acct = acct
	if err := s.saveSettingsAccount(sa); err != nil {
		log.Printf("two-step sign-in could not be saved: %v", err)
		s.showSetup(w, r, ss, http.StatusInternalServerError, acct.User, secret, T("set.save"))
		return
	}
	s.Throttle.Reset(key)
	s.note(r, "account.2fa_on", "", "user="+acct.User, false)
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
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	sa, err := s.loadSettingsAccount(ss)
	acct := sa.acct
	if err != nil || !s.canSave(sa) || acct.TOTP == "" {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	pw := r.PostFormValue("password")
	_, codeOK := webauth.VerifyTOTP(acct.TOTP, r.PostFormValue("code"), time.Now(), acct.TOTPStep)
	if len(pw) > 256 || !acct.Check(acct.User, pw) || !codeOK {
		s.Throttle.Fail(key)
		log.Printf("login failed from %s", s.clientAddr(r))
		s.showSettings(w, r, ss, http.StatusUnauthorized, settingsView{Error: T("set.2fa.off.wrong"), Which: "2fa"}, nil)
		return
	}
	acct.TOTP, acct.TOTPStep = "", 0
	sa.acct = acct
	if err := s.saveSettingsAccount(sa); err != nil {
		log.Printf("two-step sign-in could not be saved: %v", err)
		s.showSettings(w, r, ss, http.StatusInternalServerError, settingsView{Error: T("set.save"), Which: "2fa"}, nil)
		return
	}
	s.note(r, "account.2fa_off", "", "user="+acct.User, false)
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
	release, d := s.Throttle.Admit(key)
	if d > 0 {
		w.Header().Set("Retry-After", fmt.Sprint(int(d.Seconds())+1))
		s.render(w, http.StatusTooManyRequests, view{Page: "login", Need2FA: true, Pending: tok, Error: T("login.locked", int(d.Seconds())+1)})
		return
	}
	defer release()
	// Reading the last used step, checking the code against it and saving the new step are one step: two sign-ins that send the same
	// code at the same moment must not both pass.
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	acct, err := s.Account()
	var du webauth.User
	delegated := false
	if err == nil && acct.User != p.user {
		if rec, found := s.userRecord(p.user); found && !rec.Disabled {
			du, acct, delegated = rec, rec.Account(), true
		}
	}
	if err != nil || acct.User != p.user || acct.TOTP == "" {
		s.twofa.doneLogin(tok)
		expired()
		return
	}
	step, good := webauth.VerifyTOTP(acct.TOTP, code, time.Now(), acct.TOTPStep)
	if !good {
		s.Throttle.Fail(key)
		s.note(r, "login.2fa_fail", "", "user="+p.user, true)
		log.Printf("login failed from %s", s.clientAddr(r)) // read by the mpn-panel jail of fail2ban
		if !s.twofa.failLogin(tok) {
			expired()
			return
		}
		s.render(w, http.StatusUnauthorized, view{Page: "login", Need2FA: true, Pending: tok, Error: T("login.2fa.wrong")})
		return
	}
	// A code that cannot be marked as used could be used again, so then there is no session.
	acct.TOTPStep = step
	if delegated {
		du.TOTPStep = step
		if err := s.saveDelegated(du); err != nil {
			log.Printf("the used code could not be remembered, sign-in refused: %v", err)
			s.render(w, http.StatusServiceUnavailable, view{Page: "login", Need2FA: true, Pending: tok, Error: T("set.save")})
			return
		}
	} else {
		if s.SaveAccount == nil {
			s.render(w, http.StatusServiceUnavailable, view{Page: "login", Error: T("set.unavailable")})
			return
		}
		if err := s.SaveAccount(acct); err != nil {
			log.Printf("the used code could not be remembered, sign-in refused: %v", err)
			s.render(w, http.StatusServiceUnavailable, view{Page: "login", Need2FA: true, Pending: tok, Error: T("set.save")})
			return
		}
	}
	s.note(r, "login.ok", "", "user="+acct.User+" 2fa=yes", false)
	s.twofa.doneLogin(tok)
	s.Throttle.Reset(key)
	s.startSession(w, r, acct.User, p.remember)
}
