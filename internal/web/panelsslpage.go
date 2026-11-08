package web

import (
	"net/http"
	"strings"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/dnscheck"
	"mpn/internal/panelssl"
	"mpn/internal/webauth"
)

// pSSLView is what the certificate card of the security page draws; the first fields are the answer of "mpn panel ssl-info".
type pSSLView struct {
	Mode      string   `json:"mode"`
	Domain    string   `json:"domain"`
	Issuer    string   `json:"issuer"`
	Names     []string `json:"names"`
	NotBefore int64    `json:"not_before"`
	NotAfter  int64    `json:"not_after"`
	Covers    bool     `json:"covers"`
	Trusted   bool     `json:"trusted"`
	PanelPort int      `json:"panel_port"`
	PMAPort   int      `json:"pma_port"`

	Error     string `json:"-"`
	Own       bool   `json:"-"` // a certificate for a name, not the self-signed one
	UntilText string `json:"-"`
	Days      int    `json:"-"`
	Soon      bool   `json:"-"` // a certificate that was brought and expires soon: mpn does not renew it
	ServerIP  string `json:"-"`
}

func (s *Server) panelSSLRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /security/panel-ssl/letsencrypt", s.auth(s.panelSSLLetsEncrypt))
	mux.HandleFunc("POST /security/panel-ssl/custom", s.auth(s.panelSSLCustom))
	mux.HandleFunc("POST /security/panel-ssl/off", s.auth(s.panelSSLOff))
}

func (s *Server) loadPanelSSL(r *http.Request) *pSSLView {
	v := &pSSLView{ServerIP: serverIP(r.Host)}
	if e := s.list(ctl.Request{Op: "panel.sslinfo"}, v); e != "" {
		v.Error = e
		return v
	}
	v.Own = v.Mode == panelssl.ModeLetsEncrypt || v.Mode == panelssl.ModeCustom
	if v.NotAfter > 0 {
		exp := time.Unix(v.NotAfter, 0)
		v.Days = int(time.Until(exp).Hours() / 24)
		v.UntilText = exp.Local().Format("02/01/2006")
		v.Soon = v.Mode == panelssl.ModeCustom && v.Days < 30
	}
	return v
}

// panelSSLAction runs one operation on the certificate of the panel and says what a known failure means.
func (s *Server) panelSSLAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, req ctl.Request, problem string) {
	if problem != "" {
		s.refuseTo(w, ss, "/security", T("psl.fail"), problem)
		return
	}
	res := s.run(T(title), req)
	res.Back = "/security"
	if !res.OK {
		switch {
		case strings.Contains(res.Output, "site-name"):
			res.Error = T("psl.err.site")
		case strings.Contains(res.Output, dnscheck.Marker):
			res.Error = T("psl.err.dns")
		case strings.Contains(res.Output, "ssl-untrusted"):
			res.Error = T("ssl.err.untrusted")
		case strings.Contains(res.Output, "no certificate could be had") || strings.Contains(res.Output, "acme"):
			res.Error = T("ssl.err.acme")
		}
	}
	s.finish(w, r, res)
}

func panelDomain(r *http.Request) (string, string) {
	d := strings.ToLower(strings.TrimSpace(r.PostFormValue("domain")))
	if err := panelssl.ValidDomain(d); err != nil {
		return d, T("psl.err.domain")
	}
	return d, ""
}

func (s *Server) panelSSLLetsEncrypt(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	d, problem := panelDomain(r)
	s.panelSSLAction(w, r, ss, "act.psl.le", ctl.Request{Op: "panel.ssl", Domain: d}, problem)
}

func (s *Server) panelSSLCustom(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	d, problem := panelDomain(r)
	req := ctl.Request{Op: "panel.sslcustom", Domain: d, Key: strings.TrimSpace(r.PostFormValue("key")), Cert: strings.TrimSpace(r.PostFormValue("cert")),
		Chain: strings.TrimSpace(r.PostFormValue("chain")), AllowUntrusted: r.PostFormValue("allow_untrusted") == "1"}
	switch {
	case problem != "":
	case req.Cert == "":
		problem = T("ssl.err.nocert")
	case !strings.Contains(req.Cert, "BEGIN CERTIFICATE"):
		problem = T("ssl.err.notpem", T("ssl.cert"))
	case req.Key == "":
		problem = T("psl.err.nokey")
	case !strings.Contains(req.Key, "PRIVATE KEY"):
		problem = T("ssl.err.notpem", T("ssl.key"))
	case strings.Contains(req.Key, "ENCRYPTED"):
		problem = T("ssl.err.passphrase")
	}
	s.panelSSLAction(w, r, ss, "act.psl.custom", req, problem)
}

func (s *Server) panelSSLOff(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.panelSSLAction(w, r, ss, "act.psl.off", ctl.Request{Op: "panel.ssloff"}, "")
}
