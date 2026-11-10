package web

import (
	"net/http"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/webauth"
)

// saveWebmail serves the web mail under /webmail/ of one website's names, or stops. Both the program and its switch are the Email page's;
// this is only the address on the site's own name.
func (s *Server) saveWebmail(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	on := r.PostFormValue("webmail") == "1"
	res := s.run(T("act.webmail", row.Name), ctl.Request{Op: "site.webmail", Site: row.Name, Enable: on})
	res.Back = siteTab(row.Name, "")
	if !res.OK {
		switch {
		case strings.Contains(res.Output, "custom-vhost"):
			res.Error = T("vh.err.custom")
		case strings.Contains(res.Output, "webmail-needs-ssl"):
			res.Error = T("wm.err.ssl")
		case strings.Contains(res.Output, "webmail-off"):
			res.Error = T("wm.err.off")
		case strings.Contains(res.Output, "webmail-old"):
			res.Error = T("wm.err.old")
		}
	}
	s.finish(w, r, res)
}
