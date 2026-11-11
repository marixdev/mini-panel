package web

import (
	"net/http"

	"mpn/internal/ctl"
	"mpn/internal/services"
	"mpn/internal/webauth"
)

// svcGroup is one block of the page: a heading and the rows under it.
type svcGroup struct {
	Key   string
	Items []services.Item
}

// svcView is what the Services page draws.
type svcView struct {
	Groups []svcGroup
	Error  string
}

// svcOrder is the order of the blocks. A group with no rows is not drawn.
var svcOrder = []string{services.GroupService, services.GroupDatabase, services.GroupPHP, services.GroupNode, services.GroupTool}

func (s *Server) servicesPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /services", s.auth(s.servicesPage))
	// Stopping a service asks first: a stopped database or PHP-FPM takes the websites that use it down.
	mux.HandleFunc("GET /services/{key}/stop", s.auth(s.serviceStopConfirm))
	mux.HandleFunc("POST /services/{key}/{action}", s.auth(s.serviceAct))
}

func (s *Server) servicesPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v := &svcView{}
	var items []services.Item
	v.Error = s.list(ctl.Request{Op: "services.status"}, &items)
	for _, g := range svcOrder {
		grp := svcGroup{Key: g}
		for _, it := range items {
			if it.Group == g {
				grp.Items = append(grp.Items, it)
			}
		}
		if len(grp.Items) > 0 {
			v.Groups = append(v.Groups, grp)
		}
	}
	s.show(w, http.StatusOK, ss, view{Page: "services", Svc: v})
}

func (s *Server) serviceStopConfirm(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	key := r.PathValue("key")
	if services.ValidAction(key, services.Stop) != nil {
		http.NotFound(w, r)
		return
	}
	s.show(w, http.StatusOK, ss, view{Page: "confirm", Confirm: &confirm{Title: T("svc.stop.title", key), Warning: T("svc.stop.warn"),
		Button: T("svc.btn.stop"), Action: "/services/" + key + "/stop", NoType: true, Back: "/services"}})
}

// serviceAct installs a service, or starts, stops, restarts or reloads one. The key and the action are checked against the fixed list
// before anything is sent to the helper, which checks them again.
func (s *Server) serviceAct(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	key, action := r.PathValue("key"), r.PathValue("action")
	var req ctl.Request
	var title string
	switch {
	case action == "install" && services.ValidInstall(key) == nil:
		req, title = ctl.Request{Op: "services.install", SvcKey: key}, T("act.svc.install", key)
		if series := r.PostFormValue("series"); series != "" {
			// Only a database server has a version to choose. The helper checks the shape again and the plan checks the release offers it.
			if (key != "mariadb" && key != "mysql") || services.ValidSeries(series) != nil {
				s.refuseTo(w, ss, "/services", T("err.nothing"), T("svc.err.series"))
				return
			}
			req.DBSeries = series
			title = T("act.svc.install.series", key, series)
		}
	case action != "install" && services.ValidAction(key, action) == nil:
		req, title = ctl.Request{Op: "services.control", SvcKey: key, SvcAction: action}, T("act.svc."+action, key)
	default:
		s.refuseTo(w, ss, "/services", T("err.nothing"), T("svc.err.unknown"))
		return
	}
	res := s.run(title, req)
	res.Back = "/services"
	s.finish(w, r, res)
}
