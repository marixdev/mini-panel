package web

import (
	"net/http"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/webauth"
)

// saveSpeed turns compression and browser caching of static files on or off for one site.
func (s *Server) saveSpeed(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	res := s.run(T("act.speed", row.Name), ctl.Request{Op: "site.speed", Site: row.Name, Speed: r.PostFormValue("speed") == "1"})
	res.Back = siteTab(row.Name, "")
	if !res.OK && strings.Contains(res.Output, "custom-vhost") {
		res.Error = T("vh.err.custom")
	}
	s.finish(w, r, res)
}
