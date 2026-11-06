package web

import (
	"net/http"
	"strings"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/dnscheck"
	"mpn/internal/webauth"
)

// certInfo is the answer of "mpn site ssl-info".
type certInfo struct {
	Mode       string   `json:"mode"`
	Issuer     string   `json:"issuer"`
	Names      []string `json:"names"`
	NotBefore  int64    `json:"not_before"`
	NotAfter   int64    `json:"not_after"`
	Covers     bool     `json:"covers"`
	PendingCSR string   `json:"pending_csr"`
	Trusted    bool     `json:"trusted"`
}

// sslView is what the SSL tab draws.
type sslView struct {
	certInfo
	Domain           string
	ServerIP         string
	UntilText        string
	Days             int
	Soon             bool // fewer than 30 days: a certificate that was brought is not renewed by mpn
	Redirect         bool // visitors of http:// are sent to SSL
	LetsEncrypt, Own bool
	Error            string
}

func (s *Server) loadSSL(row siteRow, ip string) *sslView {
	v := &sslView{Domain: row.Primary(), ServerIP: ip, Redirect: !row.AllowHTTP}
	if e := s.list(ctl.Request{Op: "site.sslinfo", Site: row.Name}, &v.certInfo); e != "" {
		v.Error = e
		return v
	}
	v.LetsEncrypt, v.Own = v.Mode == "letsencrypt", v.Mode == "custom"
	if v.NotAfter > 0 {
		exp := time.Unix(v.NotAfter, 0)
		v.Days = int(time.Until(exp).Hours() / 24)
		v.UntilText = exp.Local().Format("02/01/2006")
		v.Soon = v.Own && v.Days < 30
	}
	return v
}

// sslAction runs one operation on the certificate of a site and shows its result on the SSL tab.
func (s *Server) sslAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, build func(row siteRow) (ctl.Request, string)) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "ssl")
	req, problem := build(row)
	if problem != "" {
		s.refuseTo(w, ss, back, T("ssl.fail"), problem)
		return
	}
	res := s.run(T(title, row.Name), req)
	res.Back = back
	if !res.OK {
		switch {
		case strings.Contains(res.Output, "custom-vhost"):
			res.Error = T("vh.err.custom")
		case strings.Contains(res.Output, dnscheck.Marker):
			res.Error = T("ssl.err.dns")
		case strings.Contains(res.Output, "ssl-untrusted"):
			res.Error = T("ssl.err.untrusted")
		case strings.Contains(res.Output, "no certificate could be had") || strings.Contains(res.Output, "acme"):
			res.Error = T("ssl.err.acme")
		}
	}
	s.finish(w, r, res)
}

func (s *Server) sslLetsEncrypt(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.sslAction(w, r, ss, "act.ssl.le", func(row siteRow) (ctl.Request, string) {
		return ctl.Request{Op: "site.ssl", Site: row.Name}, ""
	})
}

func (s *Server) sslOff(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.sslAction(w, r, ss, "act.ssl.off", func(row siteRow) (ctl.Request, string) {
		return ctl.Request{Op: "site.ssloff", Site: row.Name}, ""
	})
}

func (s *Server) sslRedirect(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.sslAction(w, r, ss, "act.ssl.redirect", func(row siteRow) (ctl.Request, string) {
		return ctl.Request{Op: "site.sslredirect", Site: row.Name, Redirect: r.PostFormValue("redirect") == "1"}, ""
	})
}

func (s *Server) sslCSR(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.sslAction(w, r, ss, "act.ssl.csr", func(row siteRow) (ctl.Request, string) {
		algo := r.PostFormValue("algo")
		if algo != "ecdsa" && algo != "rsa" {
			return ctl.Request{}, T("ssl.err.algo")
		}
		return ctl.Request{Op: "site.csr", Site: row.Name, Algo: algo}, ""
	})
}

func (s *Server) sslCustom(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.sslAction(w, r, ss, "act.ssl.custom", func(row siteRow) (ctl.Request, string) {
		req := ctl.Request{Op: "site.sslcustom", Site: row.Name, Key: strings.TrimSpace(r.PostFormValue("key")), Cert: strings.TrimSpace(r.PostFormValue("cert")),
			Chain: strings.TrimSpace(r.PostFormValue("chain")), AllowUntrusted: r.PostFormValue("allow_untrusted") == "1"}
		if req.Cert == "" {
			return req, T("ssl.err.nocert")
		}
		if !strings.Contains(req.Cert, "BEGIN CERTIFICATE") {
			return req, T("ssl.err.notpem", T("ssl.cert"))
		}
		if req.Key != "" && !strings.Contains(req.Key, "PRIVATE KEY") {
			return req, T("ssl.err.notpem", T("ssl.key"))
		}
		if strings.Contains(req.Key, "ENCRYPTED") {
			return req, T("ssl.err.passphrase")
		}
		return req, ""
	})
}
