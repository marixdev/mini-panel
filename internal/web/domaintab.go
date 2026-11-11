package web

import (
	"net/http"
	"net/url"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// domainView is what the domain card of the overview draws.
type domainView struct {
	Domain     string // without "www.", what the box holds
	Bare, With string // example.com and www.example.com, for the labels of the choices
	WWW        string
	TLS        bool
}

func newDomainView(row siteRow) *domainView {
	d := &domainView{Domain: row.Primary(), WWW: row.WWW, TLS: row.TLS}
	if row.WWW != site.WWWOff {
		d.Domain = site.BareDomain(row.Primary())
	}
	d.Bare, d.With = site.BareDomain(d.Domain), "www."+site.BareDomain(d.Domain)
	return d
}

// cleanDomain turns what a person typed into a host name: lower case, without a scheme, a path or a port.
func cleanDomain(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if i := strings.Index(v, "://"); i >= 0 {
		v = v[i+3:]
	}
	if i := strings.IndexAny(v, "/?#"); i >= 0 {
		v = v[:i]
	}
	if u, err := url.Parse("//" + v); err == nil && u.Hostname() != "" {
		v = u.Hostname()
	}
	return strings.TrimSuffix(v, ".")
}

// saveDomain changes the domain and the www choice. A new domain needs a new certificate, so the helper takes the SSL of the
// site away when the domain changes; the person chooses a source again on the SSL tab.
func (s *Server) saveDomain(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	row, status, msg := s.findSiteText(T, r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "")
	req := ctl.Request{Op: "site.domain", Site: row.Name, Domain: cleanDomain(r.PostFormValue("domain")), WWW: r.PostFormValue("www")}
	if !site.ValidDomain(req.Domain) {
		s.refuseTo(w, ss, back, T("dm.fail"), T("dm.err.domain", req.Domain))
		return
	}
	if !site.ValidWWW(req.WWW) {
		s.refuseTo(w, ss, back, T("dm.fail"), T("dm.err.www"))
		return
	}
	var rows []siteRow
	if s.listText(T, ctl.Request{Op: "site.list"}, &rows) == "" {
		for _, o := range rows {
			if o.Name == row.Name {
				continue
			}
			for _, h := range o.Hostnames {
				for _, n := range site.NamesFor(req.Domain, req.WWW) {
					if h == n {
						s.refuseTo(w, ss, back, T("dm.fail"), T("dm.err.used", n, o.Name))
						return
					}
				}
			}
		}
	}
	res := s.runText(T, T("act.site.domain", row.Name), req)
	res.Back = back
	if !res.OK && strings.Contains(res.Output, "custom-vhost") {
		res.Error = T("vh.err.custom")
	}
	s.finish(w, r, res)
}
