package web

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"mpn/internal/ctl"
	"mpn/internal/mail"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// The Email tab of a website, as in cPanel: the mail of a website is managed in the website. Its mail domains are its own names, so
// nothing has to be typed or added anywhere else; a subdomain site (blog.example.com) has mail at blog.example.com. The Email page of
// the panel keeps what belongs to the server: the mail servers, the name of the mail host and its certificate.

// mailBackKey carries, in a request, the page a mail operation returns to. The handlers of the Email page return to that page; the ones
// of a website's tab return to the tab.
type mailBackKey struct{}

func mailBackOf(r *http.Request) string {
	if b, ok := r.Context().Value(mailBackKey{}).(string); ok && b != "" {
		return b
	}
	return "/email"
}

// mailNamesOf are the mail domains a website can have: its names, once each, without a leading "www.".
func mailNamesOf(row siteRow) []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range row.Hostnames {
		d := site.BareDomain(strings.ToLower(strings.TrimSpace(h)))
		if d == "" || seen[d] || mail.ValidDomain(d) != nil {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// siteOwnsMailDomain says d is one of the mail domains of the website.
func siteOwnsMailDomain(row siteRow, d string) bool {
	d = strings.ToLower(strings.TrimSpace(d))
	for _, n := range mailNamesOf(row) {
		if n == d {
			return true
		}
	}
	return false
}

// formMailDomain is the domain a mail form is about, read the way the handlers of the Email page read it: the part after the @ of
// "address" when there is one, else of "local" joined with "domain", else "domain" alone. The check of ownership must look at the same
// value the handler acts on, or a form that carries two different ones would pass the check with one and act on the other.
func formMailDomain(r *http.Request) string {
	get := r.PostFormValue
	if r.Method == http.MethodGet {
		get = r.URL.Query().Get
	}
	if a := strings.TrimSpace(get("address")); a != "" {
		if _, d, ok := strings.Cut(a, "@"); ok {
			return strings.ToLower(strings.TrimSpace(d))
		}
		return ""
	}
	return strings.ToLower(strings.TrimSpace(get("domain")))
}

// siteMailPost runs a handler of the Email page for a website, after checking that the form is about a domain of that website. The same
// handler does the checking of the values and the work, and returns to the website's tab.
func (s *Server) siteMailPost(h func(http.ResponseWriter, *http.Request, webauth.Session)) func(http.ResponseWriter, *http.Request, webauth.Session) {
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		T := translatorForWriter(w)
		row, status, msg := s.findSiteText(T, r.PathValue("name"))
		if status != 0 {
			http.Error(w, msg, status)
			return
		}
		back := siteTab(row.Name, "email")
		if d := formMailDomain(r); d == "" || !siteOwnsMailDomain(row, d) {
			s.refuseTo(w, ss, back, T("mail.fail"), T("se.err.notyours"))
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), mailBackKey{}, back)), ss)
	}
}

type siteMailDomain struct {
	Name       string
	On         bool // set up on the mail server
	Mailboxes  []mailBoxRow
	Aliases    []mailAliasRow
	Records    []mail.Record
	Cloudflare bool
}

// siteMailView is what the Email tab of a website draws.
type siteMailView struct {
	ServerOn  bool
	Stopped   bool
	Host      string
	ServerIP  string
	Webmail   mailWebRow // the program, on the whole machine
	WebURL    string     // the website's own address for it, when the website serves it
	Domains   []siteMailDomain
	Enabled   []string // the names of the domains that are set up, for the boxes of the forms
	Error     string
	CertOwn   bool
	NoMailYet bool // the server is on but none of the names is set up
}

func (s *Server) loadSiteMail(row siteRow, hostHeader string) *siteMailView {
	return s.loadSiteMailText(T, row, hostHeader)
}

func (s *Server) loadSiteMailText(T translator, row siteRow, hostHeader string) *siteMailView {
	v := &siteMailView{ServerIP: serverIP(hostHeader)}
	st := &mailView{}
	if msg := s.listText(T, ctl.Request{Op: "mail.status"}, st); msg != "" {
		v.Error = msg
		return v
	}
	v.ServerOn, v.Host, v.Webmail = st.On, st.Host, st.Webmail
	for _, n := range []string{"postfix", "dovecot", "opendkim"} {
		if st.On && !st.Services[n] {
			v.Stopped = true
		}
	}
	if row.Webmail && row.TLS {
		v.WebURL = "https://" + row.Primary() + "/webmail/"
	}
	for _, name := range mailNamesOf(row) {
		d := siteMailDomain{Name: name}
		for _, md := range st.Domains {
			if md.Name == name {
				d.On, d.Records = true, md.Records
			}
		}
		suffix := "@" + name
		for _, b := range st.Mailboxes {
			if strings.HasSuffix(strings.ToLower(b.Address), suffix) {
				d.Mailboxes = append(d.Mailboxes, b)
			}
		}
		for _, a := range st.Aliases {
			if strings.HasSuffix(strings.ToLower(a.Address), suffix) {
				d.Aliases = append(d.Aliases, a)
			}
		}
		if d.On {
			v.Enabled = append(v.Enabled, name)
		}
		v.Domains = append(v.Domains, d)
	}
	v.NoMailYet = v.ServerOn && len(v.Enabled) == 0
	if v.ServerOn {
		var wg sync.WaitGroup
		for i := range v.Domains {
			if !v.Domains[i].On {
				continue
			}
			wg.Add(1)
			go func(d *siteMailDomain) {
				defer wg.Done()
				d.Cloudflare = s.cloudflareOfText(T, d.Name)
			}(&v.Domains[i])
		}
		wg.Wait()
	}
	return v
}
