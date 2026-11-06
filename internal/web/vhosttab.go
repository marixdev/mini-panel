package web

import (
	"net/http"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/vhost"
	"mpn/internal/webauth"
)

// vhostView is what the vHost tab draws.
type vhostView struct {
	Custom bool   `json:"custom"` // the configuration was written by hand
	Text   string `json:"text"`   // the one in use: written by hand, or made by mpn
	TLS    bool   `json:"tls"`
	MaxKB  int    `json:"-"`
	Error  string `json:"-"`
}

func (s *Server) loadVhost(row siteRow) *vhostView {
	v := &vhostView{MaxKB: vhost.MaxCustom >> 10}
	v.Error = s.list(ctl.Request{Op: "site.vhost", Site: row.Name}, v)
	return v
}

// normalizeNewlines turns the line breaks a browser sends in a text box (CRLF) into the ones a configuration file has.
func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// vhostAction runs one operation on the nginx configuration of a site and shows the result on the vHost tab.
func (s *Server) vhostAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, build func(row siteRow) (ctl.Request, string)) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "vhost")
	req, problem := build(row)
	if problem != "" {
		s.refuseTo(w, ss, back, T("vh.fail"), problem)
		return
	}
	res := s.run(T(title, row.Name), req)
	res.Back = back
	if !res.OK {
		// nginx -t says what is wrong in its own words, which the page shows below; the sentence on top says what happened to the site.
		res.Error = T("vh.err.nginx")
	}
	s.finish(w, r, res)
}

func (s *Server) saveVhost(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.vhostAction(w, r, ss, "act.vhost.save", func(row siteRow) (ctl.Request, string) {
		text := normalizeNewlines(r.PostFormValue("vhost"))
		if err := vhost.CheckCustom(text, row.TLS); err != nil {
			return ctl.Request{}, T("vh.err.check", err.Error())
		}
		return ctl.Request{Op: "site.vhostset", Site: row.Name, Vhost: text}, ""
	})
}

func (s *Server) resetVhost(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.vhostAction(w, r, ss, "act.vhost.reset", func(row siteRow) (ctl.Request, string) {
		return ctl.Request{Op: "site.vhostreset", Site: row.Name}, ""
	})
}
