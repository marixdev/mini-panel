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
	// InstalledPHP (site.InstalledPHP on a server) and NodeRoot (/opt/node) tell which versions are installed, so the form to
	// add a website offers only those. When nil or empty, every version mpn knows is offered.
	InstalledPHP func() []string
	NodeRoot     string
	// Mon takes the measurements the monitoring page draws; nil leaves the page empty. UnitState asks systemd about a service;
	// the real systemd when nil.
	Mon       *monitor.Collector
	UnitState monitor.UnitState
	// HostOS is the name of the operating system shown in the footer; /etc/os-release when empty.
	HostOS string
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
	// CreateAccount writes the first account, and fails with fs.ErrExist when an account file is there already. SetupKeyFile holds the
	// key of the setup link (see setuppage.go); without it the setup page stays closed.
	CreateAccount func(webauth.Account) error
	SetupKeyFile  string
	// accountMu makes reading the account, checking a code against it and saving the used step one step, so a code works once.
	accountMu sync.Mutex
	twofa     twoFactor
	// PanelDomainFile is the nginx file of the panel's name on port 443. When it exists and a request came to that plain address, the
	// database browser is opened under /pma/ of the name and not on its own port.
	PanelDomainFile string
	// NotifySocket is the socket of the mpn-notify service, which takes the reports of other machines when this panel is the hub of the
	// Telegram alerts; "" switches /_notify/ off.
	NotifySocket string
	// UsersFile is the file of the delegated accounts (see webauth.LoadUsers). "" switches delegated accounts off entirely.
	UsersFile string
	// Updates looks for new releases by itself; nil switches the notice and the changelog off.
	Updates *UpdateChecker
	cf      cfCache // what the name servers of mail domains said, for ten minutes

	flashMu   sync.Mutex
	flash     map[string]*result
	pairCodes map[string][2]string // a pairing code of the alerts, shown once, by session
}

const maxBody = 64 << 10

// routeMux is what the route registrars need: *http.ServeMux, or a recorder in tests that lists every pattern.
type routeMux interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	return s.guard(mux)
}

// routes registers every address of the panel. Each authenticated one must also have a rule in accessRules (access.go).
func (s *Server) routes(mux routeMux) {
	mux.HandleFunc("GET /language/{language}", s.changeLanguage)
	s.setupPages(mux)
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		if s.firstRun() {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		s.render(w, 200, view{Page: "login"})
	})
	mux.HandleFunc("POST /login", s.login)
	s.settingsPages(mux)
	s.usersPages(mux)
	mux.HandleFunc("POST /logout", s.auth(s.logout))
	mux.HandleFunc("GET /{$}", s.auth(s.sitesPage))
	s.sitePages(mux)
	s.dbPages(mux)
	s.firewallPages(mux)
	s.securityPages(mux)
	s.mailPages(mux)
	s.mailCFPages(mux)
	s.monitorPages(mux)
	s.auditPages(mux)
	s.notifyHubRoutes(mux)
	s.notifyPages(mux)
	s.servicesPages(mux)
	s.dbSettingsPages(mux)
	s.pmaRoutes(mux)
	s.backupPages(mux)
	s.sitePagesOfOne(mux)
	s.terminalPages(mux)
	s.fileRoutes(mux)
	s.assetRoutes(mux)
	mux.HandleFunc("GET /result", s.auth(s.resultPage))
	mux.HandleFunc("GET /", s.notFound) // every address no other route owns
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
		h.Set("Content-Language", requestLanguage(r))
		h.Add("Vary", "Cookie")
		h.Add("Vary", "Accept-Language")
		if s.Secure {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		// Another machine sends its report here; it carries its own key and no cookie, so a forged form could do nothing with it.
		machine := strings.HasPrefix(r.URL.Path, "/_notify/")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !machine && !sameOrigin(r) {
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

// clientAddr is the address of the visitor: what nginx says in X-Real-IP when the panel trusts it (it listens on a socket only nginx
// can open), else the address of the connection.
func (s *Server) clientAddr(r *http.Request) string {
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

// clientKey is what the limit on sign-in attempts counts by: the address, and for IPv6 its /64, because one machine is usually given a
// whole /64 and can take a new address in it for every try. The log line fail2ban reads keeps the full address (clientAddr).
func (s *Server) clientKey(r *http.Request) string {
	a := s.clientAddr(r)
	ip := net.ParseIP(a)
	if ip == nil || ip.To4() != nil {
		return a
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
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
		if !s.allowed(r, ss) {
			s.deny(w, r, ss)
			return
		}
		h(w, r, ss)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	T := translatorForWriter(w)
	key := s.clientKey(r)
	locked := func(d time.Duration) {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		s.render(w, http.StatusTooManyRequests, view{Page: "login", Error: T("login.locked", int(d.Seconds())+1)})
	}
	release, d := s.Throttle.Admit(key)
	if d > 0 {
		locked(d)
		return
	}
	defer release()
	user, pw := r.PostFormValue("user"), r.PostFormValue("password")
	acct, err := s.Account()
	// Exactly one password check runs per attempt: against the delegated user's hash when the name is one, else against the
	// administrator's. A disabled delegated user fails after the same check, so the time does not say the name exists.
	good := false
	signedAs := acct // the account whose second factor applies
	if err == nil && len(user) <= 64 && len(pw) <= 256 {
		var busy error
		if du, found := s.userRecord(user); found {
			good, busy = webauth.Account{User: du.Name, Hash: du.Hash}.CheckLimited(user, pw)
			good = good && !du.Disabled
			signedAs = du.Account()
		} else {
			good, busy = acct.CheckLimited(user, pw)
		}
		if busy != nil {
			locked(2 * time.Second)
			return
		}
	}
	if !good {
		s.Throttle.Fail(key)
		s.note(r, "login.fail", "", "user="+auditUser(user, acct.User), true)
		log.Printf("login failed from %s", s.clientAddr(r)) // read by the mpn-panel jail of fail2ban
		s.render(w, http.StatusUnauthorized, view{Page: "login", Error: T("login.wrong")})
		return
	}
	remember := r.PostFormValue("remember") == "1"
	if signedAs.TOTP != "" { // the password is right; the code of the app is still to come
		s.render(w, http.StatusOK, view{Page: "login", Need2FA: true, Pending: s.twofa.newLogin(signedAs.User, remember)})
		return
	}
	s.note(r, "login.ok", "", "user="+signedAs.User, false)
	s.Throttle.Reset(key)
	s.startSession(w, r, signedAs.User, remember)
}

// notFound answers an address the panel does not have with a page of its own. It is the same page for a signed-in person and for a
// stranger, and says nothing about what exists.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusNotFound, view{Page: "notfound"})
}

// startSession signs the browser in: a new session, and its cookie. A remembered browser gets a cookie that outlives it, so the next
// visit within 30 days needs neither the password nor the code; any other cookie ends when the browser closes.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, user string, remember bool) {
	if c, err := r.Cookie(s.cookieName()); err == nil {
		s.Sessions.Delete(c.Value) // a login never reuses an older session
	}
	tok, sess := s.Sessions.Create(user, remember)
	c := &http.Cookie{Name: s.cookieName(), Value: tok, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode}
	if d := s.Sessions.Lifetime(sess); d > 0 {
		c.MaxAge = int(d.Seconds())
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.note(r, "logout", "", "user="+ss.User, false)
	if c, err := r.Cookie(s.cookieName()); err == nil {
		s.Sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type view struct {
	Page, User, CSRF, Error, Title, Nav, Kind   string
	SetupLocked                                 bool // the setup page was opened without the key of the setup link
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
	UpNotice                                    *updateNotice // a newer release exists: the notice at the corner of every page
	Up                                          *updatePageView
	Files                                       []fileRow
	Crumbs                                      []crumb
	Parent                                      string
	Site                                        siteRow
	// Can tells a template whether the signed-in user holds at least the named role on the page's website ("view", "manage",
	// "full"). It only hides controls; the route rules are what refuse a request.
	Can func(role string) bool
	// Admin is true for the administrator's account. It hides server-wide controls on pages that have no website.
	Admin        bool
	Tab          string
	SSHEnabled   bool
	KeyStatus    string
	SSHHost      string
	ServerIP     string // shown in the header of a site
	Dom          *domainView
	SSL          *sslView
	VH           *vhostView
	Cron         *cronView
	Bg           *bgView
	Log          *logView
	Svc          *svcView
	CF           *cfView
	SM           *siteMailView
	Settings     *settingsView
	Users        *usersView
	Need2FA      bool   // the sign-in page asks for the code of the app
	Pending      string // the sign-in that waits for it
	FW           *fwView
	Audit        *auditView
	Notify       *notifyView
	Sec          *secView
	Mail         *mailView
	Cloud        *cloudView
	CloudList    *cloudListView
	WP           *wpView
	Apps         []appOption
	Locales      []string
	Mon          *monView
	DBs          []dbRow
	DBRemote     *dbRemote
	DBSet        *dbSetView
	Backups      []backupRow
	BackupKeep   int // how many backups of the site are kept
	SiteNames    []string
	PHPVersions  []string
	NodeVersions []string
	Result       *result
	Confirm      *confirm
	Foot         *footerView
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
	case strings.HasPrefix(p, "/_notify/"):
		return notifyRelayLimit
	}
	return maxBody
}
