package web

import (
	"net/http"
	"strings"
	"time"

	"mpn/internal/cloudsync"
	"mpn/internal/ctl"
	"mpn/internal/webauth"
)

type cloudPendingView struct {
	UserCode string `json:"user_code"`
	URL      string `json:"url"`
	Expires  int64  `json:"expires"`
}

type cloudStateView struct {
	At       int64  `json:"at"`
	OK       bool   `json:"ok"`
	Message  string `json:"message"`
	Uploaded int    `json:"uploaded"`
	Present  int    `json:"present"`
}

// cloudView is what the Google Drive card of the Backups tab draws; the first fields are the answer of "mpn cloud status".
type cloudView struct {
	Connected bool              `json:"connected"`
	Email     string            `json:"email"`
	Pending   *cloudPendingView `json:"pending"`
	Enabled   bool              `json:"enabled"`
	State     *cloudStateView   `json:"state"`
	Fetch     *cloudFetchView   `json:"fetch"`

	FetchAtText string `json:"-"`
	FetchMsg    string `json:"-"`
	Error       string `json:"-"`
	AtText      string `json:"-"`
	ExpiresText string `json:"-"`
	StateMsg    string `json:"-"`
	Revoked     bool   `json:"-"` // Google no longer accepts the saved permission
}

func (s *Server) cloudRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /sites/{name}/cloud/connect", s.auth(s.cloudConnect))
	mux.HandleFunc("POST /sites/{name}/cloud/finish", s.auth(s.cloudFinish))
	mux.HandleFunc("POST /sites/{name}/cloud/enable", s.auth(s.cloudEnable))
	mux.HandleFunc("POST /sites/{name}/cloud/sync", s.auth(s.cloudSync))
	mux.HandleFunc("POST /sites/{name}/cloud/disconnect", s.auth(s.cloudDisconnect))
	mux.HandleFunc("GET /sites/{name}/cloud/backups", s.auth(s.cloudBackupsPage))
	mux.HandleFunc("POST /sites/{name}/cloud/fetch", s.auth(s.cloudFetch))
}
func (s *Server) loadCloud(siteName string) *cloudView {
	return s.loadCloudText(T, siteName)
}

func (s *Server) loadCloudText(T translator, siteName string) *cloudView {
	v := &cloudView{}
	if e := s.listText(T, ctl.Request{Op: "cloud.status", Site: siteName}, v); e != "" {
		v.Error = e
		return v
	}
	if v.Pending != nil {
		v.ExpiresText = time.Unix(v.Pending.Expires, 0).Local().Format("15:04")
	}
	if v.State != nil {
		v.AtText = time.Unix(v.State.At, 0).Local().Format("02/01/2006 15:04")
		switch {
		case v.State.OK:
		case strings.HasPrefix(v.State.Message, "cloud-revoked"):
			v.Revoked = true
		default:
			v.StateMsg = v.State.Message
		}
	}
	if v.Fetch != nil {
		v.FetchAtText = time.Unix(v.Fetch.At, 0).Local().Format("02/01/2006 15:04")
		v.FetchMsg = fetchMessageText(T, v.Fetch.Message)
	}
	return v
}

// cloudAction runs one operation of the card and shows its result on the Backups tab, with the sentence for each known refusal.
func (s *Server) cloudAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, build func(name string) (ctl.Request, string)) {
	T := translatorForWriter(w)
	row, status, msg := s.findSiteText(T, r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "backups")
	req, problem := build(row.Name)
	if problem != "" {
		s.refuseTo(w, ss, back, T("cld.fail"), problem)
		return
	}
	res := s.runText(T, T(title, row.Name), req)
	res.Back = back
	if !res.OK {
		for marker, key := range map[string]string{"cloud-not-connected": "cld.err.notconnected", "cloud-pending": "cld.err.pending", "cloud-denied": "cld.err.denied",
			"cloud-expired": "cld.err.expired", "cloud-busy": "cld.err.busy", "Google refused the client": "cld.err.client", "cloud-exists": "cld.err.exists"} {
			if strings.Contains(res.Output, marker) {
				res.Error = T(key)
				break
			}
		}
	}
	s.finish(w, r, res)
}

func (s *Server) cloudConnect(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.cloudAction(w, r, ss, "act.cld.connect", func(string) (ctl.Request, string) {
		id, secret := strings.TrimSpace(r.PostFormValue("client_id")), strings.TrimSpace(r.PostFormValue("client_secret"))
		if err := cloudsync.ValidClient(id, secret); err != nil {
			return ctl.Request{}, T("cld.err.clientfmt")
		}
		return ctl.Request{Op: "cloud.connect", CloudClientID: id, CloudSecret: secret}, ""
	})
}

func (s *Server) cloudFinish(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.cloudAction(w, r, ss, "act.cld.finish", func(string) (ctl.Request, string) { return ctl.Request{Op: "cloud.finish"}, "" })
}

func (s *Server) cloudEnable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.cloudAction(w, r, ss, "act.cld.enable", func(name string) (ctl.Request, string) {
		return ctl.Request{Op: "cloud.enable", Site: name, CloudOn: r.PostFormValue("on") == "1"}, ""
	})
}

func (s *Server) cloudSync(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.cloudAction(w, r, ss, "act.cld.sync", func(name string) (ctl.Request, string) { return ctl.Request{Op: "cloud.sync", Site: name}, "" })
}

func (s *Server) cloudDisconnect(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.cloudAction(w, r, ss, "act.cld.disconnect", func(string) (ctl.Request, string) { return ctl.Request{Op: "cloud.disconnect"}, "" })
}
