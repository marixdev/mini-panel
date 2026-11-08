package web

import (
	"net/http"
	"strings"

	"mpn/internal/backup"
	"mpn/internal/ctl"
	"mpn/internal/webauth"
)

// cloudFetchView is how the last download of a backup from the drive went; the first fields are part of the answer of "mpn cloud status".
type cloudFetchView struct {
	Stamp   string `json:"stamp"`
	At      int64  `json:"at"`
	Running bool   `json:"running"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// cloudRemoteRow is a finished copy on the drive, as "mpn cloud list" prints it.
type cloudRemoteRow struct {
	Stamp  string  `json:"stamp"`
	SizeMB float64 `json:"size_mb"`
	Local  bool    `json:"local"` // the backup is on this server already
}

type cloudListView struct {
	Rows  []cloudRemoteRow
	Error string
}

// fetchMessage turns the refusals a download can end with into a sentence for the person; anything else is shown as it is.
func fetchMessage(m string) string {
	for marker, key := range map[string]string{"cloud-interrupted": "cld.fetch.interrupted", "cloud-nospace": "cld.err.nospace", "cloud-nocopy": "cld.err.nocopy",
		"cloud-busy": "cld.err.busy", "cloud-exists": "cld.err.exists", "damaged": "cld.err.damaged", "invalid_grant": "cld.revoked", "cloud-revoked": "cld.revoked"} {
		if strings.Contains(m, marker) {
			return T(key)
		}
	}
	return m
}

// cloudBackupsPage lists what the drive holds for this website and offers to bring a backup back to this server.
func (s *Server) cloudBackupsPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	v := &cloudListView{}
	if e := s.list(ctl.Request{Op: "cloud.list", Site: row.Name}, &v.Rows); e != "" {
		v.Error = fetchMessage(e)
		if strings.Contains(e, "cloud-not-connected") {
			v.Error = T("cld.err.notconnected")
		}
	}
	s.show(w, 200, ss, view{Page: "cloudlist", Site: row, Tab: "backups", CloudList: v})
}

// cloudFetch starts the download of one backup. It runs in the background, because a backup can be gigabytes.
func (s *Server) cloudFetch(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.cloudAction(w, r, ss, "act.cld.fetch", func(name string) (ctl.Request, string) {
		stamp := r.PostFormValue("backup")
		if !backup.ValidStamp(stamp) {
			return ctl.Request{}, T("err.badbackup")
		}
		return ctl.Request{Op: "cloud.fetch", Site: name, Backup: stamp}, ""
	})
}
