package web

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/mail"
	"mpn/internal/webauth"
)

type mailDomainRow struct {
	Name      string        `json:"name"`
	HasKey    bool          `json:"has_key"`
	Mailboxes int           `json:"mailboxes"`
	Aliases   int           `json:"aliases"`
	Records   []mail.Record `json:"records"`
	// Cloudflare says the name servers of the domain are Cloudflare's, so the records can be added there by the panel.
	Cloudflare bool `json:"-"`
}

type mailBoxRow struct {
	Address string `json:"address"`
	QuotaMB int    `json:"quota_mb"`
}

type mailAliasRow struct {
	Address string   `json:"address"`
	Targets []string `json:"targets"`
}

// mailWebRow is the web mail: the program that reads the mailboxes in a browser.
type mailWebRow struct {
	On        bool   `json:"on"`
	Installed bool   `json:"installed"`
	Active    bool   `json:"active"`
	Port      int    `json:"port"`
	URL       string `json:"url"` // set when the web mail has an address without a port, on the name of the mail host
}

// mailCertRow is the certificate the mail servers use: self-signed until Let's Encrypt issues one for the mail host.
type mailCertRow struct {
	Mode   string `json:"mode"`
	Days   int    `json:"days"`
	Issuer string `json:"issuer"`
	Covers bool   `json:"covers"`
}

type mailSvc struct {
	Name string
	Up   bool
}

// mailView is what the Email page draws; the first fields are the answer of "mpn mail status". It holds no password, hash or key.
type mailView struct {
	On        bool            `json:"on"`
	Host      string          `json:"host"`
	Installed bool            `json:"installed"`
	Services  map[string]bool `json:"services"`
	Ports     []int           `json:"ports"`
	Cert      mailCertRow     `json:"cert"`
	Webmail   mailWebRow      `json:"webmail"`
	Domains   []mailDomainRow `json:"domains"`
	Mailboxes []mailBoxRow    `json:"mailboxes"`
	Aliases   []mailAliasRow  `json:"aliases"`
	Error     string          `json:"-"`
	ServerIP  string          `json:"-"`
	Svc       []mailSvc       `json:"-"`
	Stopped   bool            `json:"-"` // turned on, but a server is not running
	DomainOpt []string        `json:"-"`
	WebURL    string          `json:"-"` // where the web mail is reached, on the name this page was opened with
}

func (s *Server) mailPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /email", s.auth(s.mailPage))
	mux.HandleFunc("POST /email/enable", s.auth(s.mailEnable))
	mux.HandleFunc("POST /email/disable", s.auth(s.mailDisable))
	mux.HandleFunc("POST /email/ssl", s.auth(s.mailSSL))
	mux.HandleFunc("POST /email/webmail", s.auth(s.mailWebmail))
	mux.HandleFunc("POST /email/domain/add", s.auth(s.mailDomainAdd))
	mux.HandleFunc("POST /email/domain/delete", s.auth(s.mailDomainDel))
	mux.HandleFunc("POST /email/box/add", s.auth(s.mailBoxAdd))
	mux.HandleFunc("POST /email/box/passwd", s.auth(s.mailBoxPasswd))
	mux.HandleFunc("POST /email/box/quota", s.auth(s.mailBoxQuota))
	mux.HandleFunc("POST /email/box/delete", s.auth(s.mailBoxDel))
	mux.HandleFunc("POST /email/alias/set", s.auth(s.mailAliasSet))
	mux.HandleFunc("POST /email/alias/delete", s.auth(s.mailAliasDel))
}

func (s *Server) mailPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	v := &mailView{ServerIP: serverIP(r.Host)}
	v.Error = s.listText(T, ctl.Request{Op: "mail.status"}, v)
	for _, n := range []string{"postfix", "dovecot", "opendkim"} {
		up := v.Services[n]
		v.Svc = append(v.Svc, mailSvc{n, up})
		if v.On && !up {
			v.Stopped = true
		}
	}
	for _, d := range v.Domains {
		v.DomainOpt = append(v.DomainOpt, d.Name)
	}
	s.markCloudflareText(T, v)
	if v.Webmail.On && v.Webmail.Port > 0 {
		h := r.Host
		if hh, _, err := net.SplitHostPort(r.Host); err == nil {
			h = hh
		}
		v.WebURL = "https://" + h + ":" + strconv.Itoa(v.Webmail.Port)
		if v.Webmail.URL != "" {
			v.WebURL = v.Webmail.URL
		}
	}
	s.show(w, 200, ss, view{Page: "email", Mail: v})
}

// mailAction runs one operation and shows the result, with the sentence that says what a known failure means.
func (s *Server) mailAction(w http.ResponseWriter, r *http.Request, title string, req ctl.Request) {
	T := translatorForWriter(w)
	res := s.runText(T, T(title), req)
	res.Back = mailBackOf(r)
	if !res.OK {
		switch {
		case strings.Contains(res.Output, "mail-conflict"):
			res.Error = T("mail.err.conflict")
		case strings.Contains(res.Output, "webmail-download"):
			res.Error = T("mail.err.webdl")
		case strings.Contains(res.Output, "webmail-checksum"):
			res.Error = T("mail.err.websum")
		case strings.Contains(res.Output, "webmail-nopanel"):
			res.Error = T("mail.err.webpanel")
		case strings.Contains(res.Output, "mail-nonginx"):
			res.Error = T("mail.err.nonginx")
		case strings.Contains(res.Output, "already taken"):
			res.Error = T("mail.err.exists")
		case strings.Contains(res.Output, "Unable to locate package"), strings.Contains(res.Output, "has no installation candidate"):
			res.Error = T("mail.err.nopkg")
		}
	}
	s.finish(w, r, res)
}

func (s *Server) mailRefuse(w http.ResponseWriter, r *http.Request, ss webauth.Session, msg string) {
	T := translatorForWriter(w)
	s.refuseTo(w, ss, mailBackOf(r), T("mail.fail"), msg)
}

func (s *Server) mailEnable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	host := strings.ToLower(strings.TrimSpace(r.PostFormValue("host")))
	if err := mail.ValidDomain(host); err != nil {
		s.mailRefuse(w, r, ss, T("mail.err.host"))
		return
	}
	s.mailAction(w, r, "act.mail.enable", ctl.Request{Op: "mail.enable", MailHost: host})
}

func (s *Server) mailWebmail(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.mailAction(w, r, "act.mail.webmail", ctl.Request{Op: "mail.webmail", Enable: r.PostFormValue("on") == "1"})
}

func (s *Server) mailSSL(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.mailAction(w, r, "act.mail.ssl", ctl.Request{Op: "mail.ssl"})
}

func (s *Server) mailDisable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.mailAction(w, r, "act.mail.disable", ctl.Request{Op: "mail.disable"})
}

func (s *Server) mailDomainAdd(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	d := strings.ToLower(strings.TrimSpace(r.PostFormValue("domain")))
	if err := mail.ValidDomain(d); err != nil {
		s.mailRefuse(w, r, ss, T("mail.err.domain"))
		return
	}
	s.mailAction(w, r, "act.mail.domainadd", ctl.Request{Op: "mail.domainadd", MailDomain: d})
}

func (s *Server) mailDomainDel(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	d := strings.TrimSpace(r.PostFormValue("domain"))
	if err := mail.ValidDomain(d); err != nil {
		s.mailRefuse(w, r, ss, T("mail.err.domain"))
		return
	}
	s.mailAction(w, r, "act.mail.domaindel", ctl.Request{Op: "mail.domaindel", MailDomain: d, Purge: r.PostFormValue("purge") == "1"})
}

// address joins the two boxes of a form (the part before the @ and the domain) and checks the result.
func address(r *http.Request) (string, bool) {
	a := strings.ToLower(strings.TrimSpace(r.PostFormValue("local"))) + "@" + strings.TrimSpace(r.PostFormValue("domain"))
	if r.PostFormValue("address") != "" {
		a = strings.TrimSpace(r.PostFormValue("address"))
	}
	_, _, err := mail.SplitAddress(a)
	return a, err == nil
}

// quota reads the size box: empty is no limit.
func quota(r *http.Request) (int, bool) {
	v := strings.TrimSpace(r.PostFormValue("quota"))
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	return n, err == nil && mail.ValidQuota(n) == nil
}

func (s *Server) mailBoxAdd(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	a, ok := address(r)
	if !ok {
		s.mailRefuse(w, r, ss, T("mail.err.address"))
		return
	}
	pw := r.PostFormValue("password")
	if mail.ValidPassword(pw) != nil {
		s.mailRefuse(w, r, ss, T("mail.err.password"))
		return
	}
	q, ok := quota(r)
	if !ok {
		s.mailRefuse(w, r, ss, T("mail.err.quota"))
		return
	}
	s.mailAction(w, r, "act.mail.boxadd", ctl.Request{Op: "mail.boxadd", MailAddress: a, MailPassword: pw, MailQuota: q})
}

func (s *Server) mailBoxPasswd(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	a, ok := address(r)
	if !ok {
		s.mailRefuse(w, r, ss, T("mail.err.address"))
		return
	}
	pw := r.PostFormValue("password")
	if mail.ValidPassword(pw) != nil {
		s.mailRefuse(w, r, ss, T("mail.err.password"))
		return
	}
	s.mailAction(w, r, "act.mail.boxpasswd", ctl.Request{Op: "mail.boxpasswd", MailAddress: a, MailPassword: pw})
}

func (s *Server) mailBoxQuota(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	a, ok := address(r)
	if !ok {
		s.mailRefuse(w, r, ss, T("mail.err.address"))
		return
	}
	q, ok := quota(r)
	if !ok {
		s.mailRefuse(w, r, ss, T("mail.err.quota"))
		return
	}
	s.mailAction(w, r, "act.mail.boxquota", ctl.Request{Op: "mail.boxquota", MailAddress: a, MailQuota: q})
}

func (s *Server) mailBoxDel(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	a, ok := address(r)
	if !ok {
		s.mailRefuse(w, r, ss, T("mail.err.address"))
		return
	}
	s.mailAction(w, r, "act.mail.boxdel", ctl.Request{Op: "mail.boxdel", MailAddress: a, Purge: r.PostFormValue("purge") == "1"})
}

func (s *Server) mailAliasSet(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	a, ok := address(r)
	if !ok {
		s.mailRefuse(w, r, ss, T("mail.err.address"))
		return
	}
	targets := strings.FieldsFunc(r.PostFormValue("targets"), func(c rune) bool { return c == '\n' || c == '\r' || c == ',' || c == ' ' || c == '\t' })
	if len(targets) == 0 || len(targets) > 20 {
		s.mailRefuse(w, r, ss, T("mail.err.targets"))
		return
	}
	for _, t := range targets {
		if _, _, err := mail.SplitAddress(t); err != nil {
			s.mailRefuse(w, r, ss, T("mail.err.target", t))
			return
		}
	}
	s.mailAction(w, r, "act.mail.aliasset", ctl.Request{Op: "mail.aliasset", MailAddress: a, MailTargets: targets})
}

func (s *Server) mailAliasDel(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	a, ok := address(r)
	if !ok {
		s.mailRefuse(w, r, ss, T("mail.err.address"))
		return
	}
	s.mailAction(w, r, "act.mail.aliasdel", ctl.Request{Op: "mail.aliasdel", MailAddress: a})
}
