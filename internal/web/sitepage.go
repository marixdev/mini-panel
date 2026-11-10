package web

import (
	"net/http"

	"mpn/internal/access"
	"mpn/internal/apps"
	"mpn/internal/ctl"
	"mpn/internal/dbadmin"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// siteTabLink is one entry of the menu of a site: the key names its icon and its label (tab.<key>), the path follows /sites/<name>.
type siteTabLink struct{ Key, Path string }

// siteMenu is the menu of a site, most used first. SSH/SFTP is a card of the Overview, not a tab.
func siteMenu(r siteRow) []siteTabLink {
	menu := []siteTabLink{{"overview", ""}, {"files", "/files"}, {"databases", "/databases"}, {"ssl", "/ssl"}, {"backups", "/backups"}, {"cron", "/cron"}, {"logs", "/logs"}, {"terminal", "/terminal"}, {"vhost", "/vhost"}}
	if r.App == "wordpress" { // the manager comes right after the overview: it is what a WordPress site is mostly used through
		menu = append([]siteTabLink{menu[0], {"wordpress", "/wordpress"}}, menu[1:]...)
	}
	return menu
}

func (s *Server) sitePagesOfOne(mux *http.ServeMux) {
	for _, tab := range []string{"", "/ssl", "/vhost", "/databases", "/backups", "/cron", "/logs", "/wordpress"} {
		tab := tab
		mux.HandleFunc("GET /sites/{name}"+tab, s.auth(func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
			s.siteTab(w, r, ss, map[string]string{"": "overview", "/ssl": "ssl", "/vhost": "vhost", "/databases": "databases", "/backups": "backups", "/cron": "cron", "/logs": "logs", "/wordpress": "wordpress"}[tab])
		}))
	}
	// SSH/SFTP moved into the Overview; the old address still works for bookmarks.
	mux.HandleFunc("GET /sites/{name}/ssh", s.auth(func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		if !site.ValidName(r.PathValue("name")) {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, siteTab(r.PathValue("name"), ""), http.StatusSeeOther)
	}))
	mux.HandleFunc("POST /sites/{name}/cron", s.auth(s.addCron))
	mux.HandleFunc("POST /sites/{name}/cron/{id}/toggle", s.auth(s.toggleCron))
	mux.HandleFunc("POST /sites/{name}/cron/{id}/delete", s.auth(s.deleteCron))
	mux.HandleFunc("POST /sites/{name}/vhost", s.auth(s.saveVhost))
	mux.HandleFunc("POST /sites/{name}/vhost/reset", s.auth(s.resetVhost))
	mux.HandleFunc("POST /sites/{name}/domain", s.auth(s.saveDomain))
	mux.HandleFunc("POST /sites/{name}/ssl/letsencrypt", s.auth(s.sslLetsEncrypt))
	mux.HandleFunc("POST /sites/{name}/ssl/custom", s.auth(s.sslCustom))
	mux.HandleFunc("POST /sites/{name}/ssl/csr", s.auth(s.sslCSR))
	mux.HandleFunc("POST /sites/{name}/ssl/off", s.auth(s.sslOff))
	mux.HandleFunc("POST /sites/{name}/ssl/redirect", s.auth(s.sslRedirect))
	mux.HandleFunc("POST /sites/{name}/settings", s.auth(s.saveSettings))
	mux.HandleFunc("POST /sites/{name}/speed", s.auth(s.saveSpeed))
	s.cloudRoutes(mux)
	s.appRoutes(mux)
	s.wpRoutes(mux)
	mux.HandleFunc("POST /sites/{name}/ssh/enable", s.auth(s.sshEnable))
	mux.HandleFunc("GET /sites/{name}/ssh/reset", s.auth(s.sshConfirm("reset")))
	mux.HandleFunc("POST /sites/{name}/ssh/reset", s.auth(s.sshAct("reset")))
	mux.HandleFunc("GET /sites/{name}/ssh/disable", s.auth(s.sshConfirm("disable")))
	mux.HandleFunc("POST /sites/{name}/ssh/disable", s.auth(s.sshAct("disable")))
	mux.HandleFunc("POST /sites/{name}/ssh/panelkey", s.auth(s.sshPanelKey))
}

// findSite looks the site up in the helper's list. The name in the address is checked first, so only a valid name is
// ever sent on.
func (s *Server) findSite(name string) (siteRow, int, string) {
	if !site.ValidName(name) {
		return siteRow{}, http.StatusNotFound, T("err.nosite")
	}
	var rows []siteRow
	if e := s.list(ctl.Request{Op: "site.list"}, &rows); e != "" {
		return siteRow{}, http.StatusBadGateway, e
	}
	for _, row := range rows {
		if row.Name == name {
			return row, 0, ""
		}
	}
	return siteRow{}, http.StatusNotFound, T("err.nosite")
}

func (s *Server) passwdFile() string {
	if s.PasswdFile != "" {
		return s.PasswdFile
	}
	return "/etc/passwd"
}

func (s *Server) siteTab(w http.ResponseWriter, r *http.Request, ss webauth.Session, tab string) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	v := view{Page: "site", Site: row, Tab: tab, ServerIP: serverIP(r.Host)}
	switch tab {
	case "overview":
		v.Dom = newDomainView(row)
		var e string
		if v.Over, e = s.loadOverview(row); e != "" {
			v.Error = e
		}
		s.loadSSH(&v, r, row)
		v.Apps, v.Locales = appOptions(), apps.Locales
	case "ssl":
		v.SSL = s.loadSSL(row, v.ServerIP)
	case "vhost":
		v.VH = s.loadVhost(row)
	case "cron":
		v.Cron = s.loadCron(row)
	case "logs":
		v.Log = s.loadLogs(r, row)
	case "databases":
		v.DBBase = dbadmin.BaseName(row.Name)
		v.PMA = s.pmaOn()
		v.Error = s.list(ctl.Request{Op: "db.list", Site: row.Name}, &v.DBs)
	case "backups":
		v.Error = s.list(ctl.Request{Op: "backup.list", Site: row.Name}, &v.Backups)
		v.Cloud = s.loadCloud(row.Name)
	case "wordpress":
		if row.App != "wordpress" {
			http.NotFound(w, r)
			return
		}
		v.WP = s.loadWP(row, r)
	}
	s.show(w, 200, ss, v)
}

// loadSSH fills in what the SSH/SFTP card of the Overview shows.
func (s *Server) loadSSH(v *view, r *http.Request, row siteRow) {
	v.SSHHost = hostOnly(r.Host)
	v.SSHEnabled = access.Enabled(s.passwdFile(), row.User)
	v.KeyStatus = "unknown"
	if v.SSHEnabled && s.LoginCheck != nil {
		v.KeyStatus = "ok"
		if err := s.LoginCheck(row.User); err != nil {
			v.KeyStatus = "broken"
		}
	}
}

// loginResult runs the helper and, if it worked, adds the account and the new password to what the page shows once.
func (s *Server) loginResult(r *http.Request, title string, row siteRow, password string) *result {
	res := s.run(title, ctl.Request{Op: "site.access", Site: row.Name, Enable: true, Password: password})
	res.Back = siteTab(row.Name, "")
	if res.OK {
		res.Output += "\n\n" + T("res.access", hostOnly(r.Host), row.User, password)
	}
	return res
}

func (s *Server) sshEnable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	password, err := dbadmin.NewPassword()
	if err != nil {
		s.refuseTo(w, ss, siteTab(row.Name, ""), T("err.nothing"), err.Error())
		return
	}
	s.finish(w, r, s.loginResult(r, T("act.ssh.on", row.Name), row, password))
}

var sshActions = map[string]struct{ title, warn, button, act string }{
	"reset":   {"ssh.reset.title", "ssh.reset.warn", "ssh.reset.btn", "act.ssh.reset"},
	"disable": {"ssh.disable.title", "ssh.disable.warn", "ssh.disable.btn", "act.ssh.off"},
}

func (s *Server) sshConfirm(action string) func(http.ResponseWriter, *http.Request, webauth.Session) {
	a := sshActions[action]
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		row, status, msg := s.findSite(r.PathValue("name"))
		if status != 0 {
			http.Error(w, msg, status)
			return
		}
		s.show(w, 200, ss, view{Page: "confirm", Confirm: &confirm{Title: T(a.title, row.Name), Warning: T(a.warn), Button: T(a.button),
			Action: "/sites/" + row.Name + "/ssh/" + action, Expect: row.Name, Back: "/sites/" + row.Name}})
	}
}

func (s *Server) sshAct(action string) func(http.ResponseWriter, *http.Request, webauth.Session) {
	a := sshActions[action]
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		row, status, msg := s.findSite(r.PathValue("name"))
		if status != 0 {
			http.Error(w, msg, status)
			return
		}
		if r.PostFormValue("confirm") != row.Name {
			s.refuseTo(w, ss, siteTab(row.Name, ""), T("err.nothing"), T("err.confirm"))
			return
		}
		if action == "disable" {
			res := s.run(T(a.act, row.Name), ctl.Request{Op: "site.access", Site: row.Name})
			res.Back = siteTab(row.Name, "")
			s.finish(w, r, res)
			return
		}
		password, err := dbadmin.NewPassword()
		if err != nil {
			s.refuseTo(w, ss, siteTab(row.Name, ""), T("err.nothing"), err.Error())
			return
		}
		s.finish(w, r, s.loginResult(r, T(a.act, row.Name), row, password))
	}
}

func (s *Server) sshPanelKey(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	res := s.run(T("act.ssh.key", row.Name), ctl.Request{Op: "site.panelkey", Site: row.Name})
	res.Back = siteTab(row.Name, "")
	s.finish(w, r, res)
}
