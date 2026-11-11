package web

import (
	"net/http"
	"strconv"

	"mpn/internal/backup"
	"mpn/internal/ctl"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// backupRow is one line of `mpn backup list -json`.
type backupRow struct {
	Site, Backup, State string
	Files               bool
	Databases           []string
	SizeMB              float64 `json:"size_mb"`
}

func (s *Server) backupPages(mux *http.ServeMux) {
	// The list of backups is the Backups tab of each website; only the actions have addresses of their own.
	mux.HandleFunc("POST /backups/create", s.auth(s.createBackup))
	mux.HandleFunc("GET /backups/restore", s.auth(s.restoreConfirm))
	mux.HandleFunc("POST /backups/restore", s.auth(s.restoreBackup))
	mux.HandleFunc("POST /backups/prune", s.auth(s.pruneBackups))
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	name := r.PostFormValue("site")
	res := s.runText(T, T("act.bk.create", name), ctl.Request{Op: "backup.create", Site: name})
	res.Back = safeBack(r.PostFormValue("back"), "/")
	s.finish(w, r, res)
}

func (s *Server) refuse(w http.ResponseWriter, ss webauth.Session, title, msg string) {
	s.refuseTo(w, ss, "/", title, msg)
}

// refuseTo shows why nothing was done, with a button back to the page the person came from.
func (s *Server) refuseTo(w http.ResponseWriter, ss webauth.Session, back, title, msg string) {
	s.show(w, http.StatusBadRequest, ss, view{Page: "result", Result: &result{Title: title, Error: msg, Back: back}})
}

func (s *Server) restoreConfirm(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	name, stamp := r.URL.Query().Get("site"), r.URL.Query().Get("backup")
	if !site.ValidName(name) || !backup.ValidStamp(stamp) {
		http.Error(w, T("err.badbackup"), http.StatusBadRequest)
		return
	}
	var rows []backupRow
	if e := s.listText(T, ctl.Request{Op: "backup.list", Site: name}, &rows); e != "" {
		s.show(w, http.StatusBadGateway, ss, view{Page: "result", Result: &result{Title: T("bk.rs.fail"), Error: e, Back: safeBack(r.URL.Query().Get("back"), "/backups")}})
		return
	}
	for _, row := range rows {
		if row.Backup != stamp || row.State != "ok" {
			continue
		}
		var checks []check
		if row.Files {
			checks = append(checks, check{Name: "files", Value: "on", Label: T("bk.rs.files"), On: true})
		}
		for _, d := range row.Databases {
			checks = append(checks, check{Name: "database", Value: d, Label: T("bk.rs.db", d), On: true})
		}
		s.show(w, 200, ss, view{Page: "confirm", Confirm: &confirm{
			Title: T("bk.rs.title", name, stamp), Back: safeBack(r.URL.Query().Get("back"), "/backups"), Action: "/backups/restore", Expect: name, Button: T("bk.rs.btn"), Checks: checks,
			Warning: T("bk.rs.warn"),
			Hidden:  map[string]string{"site": name, "backup": stamp, "back": safeBack(r.URL.Query().Get("back"), "/backups")}}})
		return
	}
	http.Error(w, T("err.nobackup"), http.StatusNotFound)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	name, stamp := r.PostFormValue("site"), r.PostFormValue("backup")
	files, dbs := r.PostFormValue("files") != "", r.PostForm["database"]
	back := safeBack(r.PostFormValue("back"), "/backups")
	switch {
	case !site.ValidName(name) || !backup.ValidStamp(stamp) || r.PostFormValue("confirm") != name:
		s.refuseTo(w, ss, back, T("bk.rs.fail"), T("err.confirm"))
	case !files && len(dbs) == 0:
		s.refuseTo(w, ss, back, T("bk.rs.fail"), T("err.pick"))
	default:
		req := ctl.Request{Op: "backup.restore", Site: name, Backup: stamp, Files: files, Databases: dbs}
		res := s.runText(T, T("act.bk.restore", name, stamp), req)
		res.Back = back
		s.finish(w, r, res)
	}
}

func (s *Server) pruneBackups(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	name := r.PostFormValue("site")
	keep, err := strconv.Atoi(r.PostFormValue("keep"))
	back := safeBack(r.PostFormValue("back"), "/backups")
	switch {
	case !site.ValidName(name) || r.PostFormValue("confirm") != name:
		s.refuseTo(w, ss, back, T("bk.pr.fail"), T("err.confirm"))
	case err != nil || keep < 1 || keep > 365:
		s.refuseTo(w, ss, back, T("bk.pr.fail"), T("err.keep"))
	default:
		res := s.runText(T, T("act.bk.prune", name), ctl.Request{Op: "backup.prune", Site: name, Keep: keep})
		res.Back = back
		s.finish(w, r, res)
	}
}
