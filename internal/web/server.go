// Package web is the panel's HTTP side. It runs without privileges and never touches the machine itself: every change
// is a ctl.Request sent to the root helper.
package web

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pkg/sftp"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/monitor"
	"mpn/internal/webauth"
)

type Server struct {
	Account  func() (webauth.Account, error) // read on every login, so a new password applies at once
	Sessions *webauth.Sessions
	Throttle *webauth.Throttle
	Hosts    []string // names the panel answers to, besides localhost; any other Host header is refused
	// HostsFile lists more such names, one per line. It is read while the panel runs, so a name added by the SSL page is
	// accepted at once.
	HostsFile  string
	Secure     bool // the panel is reached over HTTPS: sets the Secure cookie flag and HSTS
	TrustProxy bool // take the client address from X-Real-IP, which the local nginx sets
	// Call sends a request to the root helper. The web side has no other way to change anything.
	Call func(ctl.Request) (ctl.Response, error)
	// UserExists says whether the machine already has an account with that name; the system's user database when nil.
	UserExists func(string) bool
	// LoginCheck says whether the panel can log in as that account with its own key; nil means it is not checked.
	LoginCheck func(user string) error
	// PasswdFile is where the shell of an account is read from; /etc/passwd when empty.
	PasswdFile string
	// PHPDir (where php-fpm<version> programs are, /usr/sbin) and NodeRoot (/opt/node) tell which versions are installed, so
	// the form to add a website offers only those. When empty, every version mpn knows is offered.
	PHPDir, NodeRoot string
	// Mon takes the measurements the monitoring page draws; nil leaves the page empty. UnitState asks systemd about a service;
	// the real systemd when nil.
	Mon       *monitor.Collector
	UnitState monitor.UnitState
	// Dialer logs in as a website's account for the terminal; nil switches the terminal off.
	Dialer            Dialer
	TermIdle, TermMax time.Duration // a terminal closes after this long without typing, and after this long in all
	MaxTerminals      int           // open terminals at once; 8 when 0
	TermLog           func(event, site string, err error)
	terms             atomic.Int32
	// PMAPort is the port of the database browser (phpMyAdmin) on this machine, 0 when it is not installed. PMAKey is the
	// key that its sign-on page proves itself with; the browser is off when the key is shorter than 32 bytes.
	PMAPort int
	PMAKey  []byte
	tickets pmaTickets
	// MaxUpload is the largest file the file manager takes in one upload; 1 GiB when 0.
	MaxUpload int64
	// FMRoot finds the home directory the file manager works in; the SFTP session's start directory when nil.
	FMRoot func(*sftp.Client) (string, error)

	// SaveAccount writes the administrator's account (a changed password, the secret of the two-step sign-in). The account file
	// belongs to the panel's own account, so no help from the root helper is needed; nil switches those settings off.
	SaveAccount func(webauth.Account) error
	twofa       twoFactor
	// PanelDomainFile is the nginx file of the panel's name on port 443. When it exists and a request came to that plain address, the
	// database browser is opened under /pma/ of the name and not on its own port.
	PanelDomainFile string

	flashMu sync.Mutex
	flash   map[string]*result
}

const maxBody = 64 << 10

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) { s.render(w, 200, view{Page: "login"}) })
	mux.HandleFunc("POST /login", s.login)
	s.settingsPages(mux)
	mux.HandleFunc("POST /logout", s.auth(s.logout))
	mux.HandleFunc("GET /{$}", s.auth(s.sitesPage))
	s.sitePages(mux)
	s.dbPages(mux)
	s.firewallPages(mux)
	s.securityPages(mux)
	s.mailPages(mux)
	s.monitorPages(mux)
	s.servicesPages(mux)
	s.pmaRoutes(mux)
	s.backupPages(mux)
	s.sitePagesOfOne(mux)
	s.terminalPages(mux)
	s.fileRoutes(mux)
	s.assetRoutes(mux)
	mux.HandleFunc("GET /result", s.auth(s.resultPage))
	return s.guard(mux)
}

// guard runs before every handler: it refuses a Host header the panel does not own (which stops DNS rebinding), refuses
// a form that did not come from the panel's own pages, limits the body, and sets the security headers.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostOK(r.Host) {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", basePolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		if s.Secure {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !sameOrigin(r) {
				http.Error(w, "cross-site request refused", http.StatusForbidden)
				return
			}
			if limit := bodyLimit(r.URL.Path); limit > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostOK(hostport string) bool {
	host := strings.ToLower(hostport)
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = strings.ToLower(h)
	}
	if host == "localhost" {
		return true
	}
	// An IP address is always accepted: DNS rebinding works through a name an attacker controls, and such a name
	// arrives as that name in the Host header, never as an address.
	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		return true
	}
	for _, x := range s.Hosts {
		if host == strings.ToLower(x) {
			return true
		}
	}
	for _, x := range s.fileHosts() {
		if host == x {
			return true
		}
	}
	return false
}

// sameOrigin accepts a request whose Origin is this very host, or that the browser itself marks as same-origin.
func sameOrigin(r *http.Request) bool {
	// Browsers send "Origin: null" for a form post from the page's own origin when the referrer policy hides the
	// referrer. Such a request is accepted only when the browser itself marks it same-origin, which a page on another
	// site cannot forge.
	if o := r.Header.Get("Origin"); o != "" && o != "null" {
		u, err := url.Parse(o)
		return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
	}
	return r.Header.Get("Sec-Fetch-Site") == "same-origin"
}

func (s *Server) cookieName() string {
	if s.Secure {
		return "__Host-mpn"
	}
	return "mpn"
}

func (s *Server) clientKey(r *http.Request) string {
	if s.TrustProxy {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// auth wraps a handler that needs a logged-in user. A form post must also carry the session's CSRF token.
func (s *Server) auth(h func(http.ResponseWriter, *http.Request, webauth.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.cookieName())
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ss, ok := s.Sessions.Get(c.Value)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && !s.Sessions.CheckCSRF(c.Value, r.PostFormValue("csrf")) {
			http.Error(w, "invalid form token; go back, reload the page and try again", http.StatusForbidden)
			return
		}
		h(w, r, ss)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	key := s.clientKey(r)
	if d := s.Throttle.Wait(key); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		s.render(w, http.StatusTooManyRequests, view{Page: "login", Error: T("login.locked", int(d.Seconds())+1)})
		return
	}
	user, pw := r.PostFormValue("user"), r.PostFormValue("password")
	acct, err := s.Account()
	if err != nil || len(user) > 64 || len(pw) > 256 || !acct.Check(user, pw) {
		s.Throttle.Fail(key)
		log.Printf("login failed from %s", key) // read by the mpn-panel jail of fail2ban
		s.render(w, http.StatusUnauthorized, view{Page: "login", Error: T("login.wrong")})
		return
	}
	if acct.TOTP != "" { // the password is right; the code of the app is still to come
		s.render(w, http.StatusOK, view{Page: "login", Need2FA: true, Pending: s.twofa.newLogin(acct.User)})
		return
	}
	s.Throttle.Reset(key)
	s.startSession(w, r, acct.User)
}

// startSession signs the browser in: a new session, and its cookie.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user string) {
	if c, err := r.Cookie(s.cookieName()); err == nil {
		s.Sessions.Delete(c.Value) // a login never reuses an older session
	}
	tok, _ := s.Sessions.Create(user)
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: tok, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, _ webauth.Session) {
	if c, err := r.Cookie(s.cookieName()); err == nil {
		s.Sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type view struct {
	Page, User, CSRF, Error, Title, Nav, Kind   string
	Sites                                       []siteRow
	Kinds                                       []string
	Reserved                                    string
	Password                                    string
	Nonce                                       string
	Dir, FileName, EditPath, EditText, FileKind string
	DBBase                                      string    // the start every database name of the site has
	PMA                                         bool      // the database browser is installed: show its button
	Over                                        *overview // the overview tab of a site: its state and its settings
	PMALink, PMAName                            string    // where the interstitial page leads, and the database it opens
	EditMtime                                   int64
	CRLF, Cut                                   bool
	Notice                                      *result
	Files                                       []fileRow
	Crumbs                                      []crumb
	Parent                                      string
	Site                                        siteRow
	Tab                                         string
	SSHEnabled                                  bool
	KeyStatus                                   string
	SSHHost                                     string
	ServerIP                                    string // shown in the header of a site
	Dom                                         *domainView
	SSL                                         *sslView
	VH                                          *vhostView
	Cron                                        *cronView
	Log                                         *logView
	Svc                                         *svcView
	Settings                                    *settingsView
	Need2FA                                     bool   // the sign-in page asks for the code of the app
	Pending                                     string // the sign-in that waits for it
	FW                                          *fwView
	Sec                                         *secView
	Mail                                        *mailView
	Cloud                                       *cloudView
	CloudList                                   *cloudListView
	WP                                          *wpView
	Apps                                        []appOption
	Locales                                     []string
	Mon                                         *monView
	DBs                                         []dbRow
	Backups                                     []backupRow
	SiteNames                                   []string
	PHPVersions                                 []string
	NodeVersions                                []string
	Result                                      *result
	Confirm                                     *confirm
}

func (s *Server) render(w http.ResponseWriter, status int, v view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	s.execute(w, v)
}

// bodyLimit is how much of a request body the panel reads. Forms are small. An upload is read as a stream by its own
// handler, which stops at its own limit, and the text of the editor may be as large as the largest file it opens.
func bodyLimit(p string) int64 {
	switch {
	case strings.HasPrefix(p, "/sites/") && strings.HasSuffix(p, "/files/upload"):
		return 0
	case strings.HasPrefix(p, "/sites/") && strings.HasSuffix(p, "/files/save"):
		return 6 << 20
	case strings.HasPrefix(p, "/sites/") && strings.HasSuffix(p, "/vhost"):
		return 256 << 10 // up to 48 KB of configuration; the encoding of a form can make every character three bytes long
	case strings.HasPrefix(p, "/sites/") && strings.HasSuffix(p, "/ssl/custom"):
		return 128 << 10 // three boxes of up to 16 KB, which the encoding of the form makes larger
	}
	return maxBody
}
