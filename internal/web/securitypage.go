package web

import (
	"net/http"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/security"
	"mpn/internal/webauth"
)

type secJail struct {
	Name        string   `json:"name"`
	Failed      int      `json:"failed"`
	TotalFailed int      `json:"total_failed"`
	Banned      int      `json:"banned"`
	TotalBanned int      `json:"total_banned"`
	IPs         []string `json:"ips"`
}

// secView is what the security page draws; the first fields are the answer of "mpn security status".
type secView struct {
	WAFMode      string    `json:"waf_mode"`
	WAFInstalled bool      `json:"waf_installed"`
	CRSVersion   string    `json:"crs_version"`
	F2BOn        bool      `json:"f2b_on"`
	F2BInstalled bool      `json:"f2b_installed"`
	F2BActive    bool      `json:"f2b_active"`
	Ignore       []string  `json:"ignore"`
	Jails        []secJail `json:"jails"`
	Error        string    `json:"-"`
	Client       string    `json:"-"`
	IgnoreText   string    `json:"-"`
	PSSL         *pSSLView `json:"-"`
}

func (s *Server) securityPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /security", s.auth(s.securityPage))
	mux.HandleFunc("POST /security/waf", s.auth(s.secWAF))
	mux.HandleFunc("POST /security/f2b", s.auth(s.secF2B))
	mux.HandleFunc("POST /security/unban", s.auth(s.secUnban))
	s.panelSSLRoutes(mux)
}

func (s *Server) securityPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v := &secView{Client: s.clientKey(r)}
	v.Error = s.list(ctl.Request{Op: "security.status"}, v)
	v.IgnoreText = strings.Join(v.Ignore, "\n")
	v.PSSL = s.loadPanelSSL(r)
	s.show(w, 200, ss, view{Page: "security", Sec: v})
}

// secAction runs one operation and shows the result, with the sentence that says what a known failure means.
func (s *Server) secAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, req ctl.Request) {
	res := s.run(T(title), req)
	res.Back = "/security"
	if !res.OK {
		switch {
		case strings.Contains(res.Output, "Core Rule Set is not installed"):
			res.Error = T("sec.err.crs")
		case strings.Contains(res.Output, "Unable to locate package"), strings.Contains(res.Output, "has no installation candidate"):
			res.Error = T("sec.err.nopkg")
		}
	}
	s.finish(w, r, res)
}

func (s *Server) secWAF(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	mode := r.PostFormValue("mode")
	if err := security.ValidMode(mode); err != nil {
		s.refuseTo(w, ss, "/security", T("sec.fail"), T("sec.err.mode"))
		return
	}
	s.secAction(w, r, ss, "act.sec.waf", ctl.Request{Op: "security.waf", SecMode: mode})
}

// secF2B turns fail2ban on or off. The address of the person is added to the trusted list when it is turned on, so the person
// who just typed a wrong password three times is not the first one to be banned.
func (s *Server) secF2B(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	on := r.PostFormValue("on") == "1"
	list := strings.FieldsFunc(r.PostFormValue("ignore"), func(c rune) bool { return c == '\n' || c == '\r' || c == ',' || c == ' ' || c == '\t' })
	if on && s.clientKey(r) != "" {
		list = append(list, s.clientKey(r))
	}
	clean, err := security.ValidIgnore(list)
	if err != nil {
		s.refuseTo(w, ss, "/security", T("sec.fail"), T("sec.err.ignore", err.Error()))
		return
	}
	s.secAction(w, r, ss, "act.sec.f2b", ctl.Request{Op: "security.f2b", SecOn: on, SecIgnore: clean})
}

func (s *Server) secUnban(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	jail, ip := r.PostFormValue("jail"), r.PostFormValue("ip")
	if security.ValidJail(jail) != nil {
		s.refuseTo(w, ss, "/security", T("sec.fail"), T("sec.err.jail"))
		return
	}
	if _, err := security.ValidAddr(ip); err != nil {
		s.refuseTo(w, ss, "/security", T("sec.fail"), T("sec.err.ip"))
		return
	}
	s.secAction(w, r, ss, "act.sec.unban", ctl.Request{Op: "security.unban", Jail: jail, IP: ip})
}
