package web

import (
	"net/http"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/tune"
	"mpn/internal/webauth"
)

// dbSetView is the page of the settings of the database server: a field for each setting the panel offers, the value the panel set
// (empty when it is the server's own), and what the running server says now.
type dbSetView struct {
	Engine string
	Rows   []dbSetRow
	Error  string
}

type dbSetRow struct {
	tune.DBSetting
	Value, Current string
}

func (s *Server) dbSettingsPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /services/database/settings", s.auth(s.dbSettingsPage))
	mux.HandleFunc("POST /services/database/settings", s.auth(s.dbSettingsSave))
}

func (s *Server) loadDBSettings(T translator) *dbSetView {
	var info struct {
		Engine   string            `json:"engine"`
		Settings []tune.DBSetting  `json:"settings"`
		Values   map[string]string `json:"values"`
		Current  map[string]string `json:"current"`
	}
	v := &dbSetView{}
	if e := s.listText(T, ctl.Request{Op: "db.settingsinfo"}, &info); e != "" {
		v.Error = e
		return v
	}
	v.Engine = info.Engine
	for _, st := range info.Settings {
		v.Rows = append(v.Rows, dbSetRow{DBSetting: st, Value: info.Values[st.Key], Current: info.Current[st.Key]})
	}
	return v
}

func (s *Server) dbSettingsPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.show(w, http.StatusOK, ss, view{Page: "dbsettings", DBSet: s.loadDBSettings(T)})
}

func (s *Server) dbSettingsSave(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	values := map[string]string{}
	for _, st := range tune.DBSettings {
		values[st.Key] = strings.TrimSpace(r.PostFormValue(st.Key))
	}
	if _, err := tune.CheckDBValues(values); err != nil {
		v := s.loadDBSettings(T)
		for i := range v.Rows {
			v.Rows[i].Value = values[v.Rows[i].Key]
		}
		v.Error = T("dbset.err.value", err.Error())
		s.show(w, http.StatusBadRequest, ss, view{Page: "dbsettings", DBSet: v})
		return
	}
	res := s.runText(r, T, T("act.dbset"), ctl.Request{Op: "db.settings", DBValues: values})
	res.Back = "/services/database/settings"
	if !res.OK && strings.Contains(res.Output, "db-settings-refused") {
		res.Error = T("dbset.err.refused")
	}
	s.finish(w, r, res)
}
