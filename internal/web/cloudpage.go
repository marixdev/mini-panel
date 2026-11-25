package web

import (
	"net/http"
	"strings"
	"time"

	"mpn/internal/cloudprov"
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

// cloudView is what the card "Off-site backup" of the Backups tab draws; the first fields are the answer of "mpn cloud status".
type cloudView struct {
	Connected bool              `json:"connected"`
	Kind      string            `json:"kind"` // "gdrive", or the type of the storage such as "s3"
	Email     string            `json:"email"`
	Where     string            `json:"where"`
	Pending   *cloudPendingView `json:"pending"`
	Enabled   bool              `json:"enabled"`
	State     *cloudStateView   `json:"state"`
	Fetch     *cloudFetchView   `json:"fetch"`

	Forms       []cloudForm `json:"-"`
	KindName    string      `json:"-"` // the name of a storage generated from rclone's list, which has no translation
	FetchAtText string      `json:"-"`
	FetchMsg    string      `json:"-"`
	Error       string      `json:"-"`
	AtText      string      `json:"-"`
	ExpiresText string      `json:"-"`
	StateMsg    string      `json:"-"`
	Revoked     bool        `json:"-"` // Google no longer accepts the saved permission
}

// cloudForm is the form of one kind of storage, drawn from the list in internal/cloudprov.
type cloudForm struct {
	Type   string
	Bucket bool
	Fields []cloudField
	// Name is the name shown for a storage without a form of its own (one generated from rclone's list); the others are
	// translated by their type. Desc is a key of the translations.
	Name, Desc string
	Generated  bool
	Search     string // what the search box of the list matches, in lower case
}

// providerNames tidies the names rclone gives some backends: lower case, a list of other brands, or a long explanation.
var providerNames = map[string]string{
	"drive": "Google Drive (rclone token)", "seafile": "Seafile", "pcloud": "pCloud", "dosya": "Dosya", "premiumizeme": "premiumize.me",
	"koofr": "Koofr / Digi Storage", "hdfs": "Hadoop HDFS", "swift": "OpenStack Swift", "iclouddrive": "iCloud Drive",
	"pixeldrain": "Pixeldrain", "filelu": "FileLu", "shade": "Shade", "sia": "Sia", "storj": "Storj", "quatrix": "Quatrix",
	"qingstor": "QingStor", "oracleobjectstorage": "Oracle Cloud Object Storage", "netstorage": "Akamai NetStorage",
	"sharefile": "Citrix ShareFile", "sugarsync": "SugarSync", "azureblob": "Azure Blob Storage", "azurefiles": "Azure Files",
	"ulozto": "Uloz.to", "filefabric": "Enterprise File Fabric", "smb": "SMB / CIFS (Windows share)", "mailru": "Mail.ru Cloud",
	"internxt": "Internxt Drive", "kdrive": "Infomaniak kDrive", "googlecloudstorage": "Google Cloud Storage",
}

func providerName(p cloudprov.Provider) string {
	if n, ok := providerNames[p.Type]; ok {
		return n
	}
	if p.Name != "" {
		return p.Name
	}
	return p.Type
}

type cloudField struct {
	cloudprov.Field
	Label       string // a key of the translations
	Placeholder string
}

// placeholders are examples of what goes in a box. They are the same in every language.
var placeholders = map[string]string{
	"s3.access_key_id": "AKIA… / 0123abcd…", "s3.endpoint": "https://<account>.r2.cloudflarestorage.com", "s3.region": "auto",
	"b2.account": "0012a3b4c5d6", "sftp.host": "backup.example.com", "sftp.user": "backup",
	"sftp.host_keys": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA…", "webdav.url": "https://cloud.example.com/remote.php/dav/files/me/",
	"webdav.user": "me", "ftp.host": "ftp.example.com", "ftp.user": "backup",
}

func cloudForms() []cloudForm {
	out := make([]cloudForm, 0, len(cloudprov.Providers))
	for _, p := range cloudprov.Providers {
		f := cloudForm{Type: p.Type, Bucket: p.Bucket, Generated: p.Generated, Desc: "cld.kind." + p.Type + ".d"}
		if p.Generated {
			f.Name, f.Desc = providerName(p), "cld.gen.d"
			if p.OAuth {
				f.Desc = "cld.gen.oauth.d"
			}
		}
		f.Search = strings.ToLower(p.Type + " " + f.Name + " " + p.Name)
		for _, x := range p.Fields {
			label := "cld.f." + p.Type + "." + x.Key
			if p.Generated {
				label = x.Help // rclone's own words, in English; T gives back a text it has no translation for
				if label == "" {
					label = x.Key
				}
			}
			ph := placeholders[p.Type+"."+x.Key]
			if ph == "" {
				ph = x.Placeholder
			}
			f.Fields = append(f.Fields, cloudField{Field: x, Label: label, Placeholder: ph})
		}
		out = append(out, f)
	}
	return out
}

func (s *Server) cloudRoutes(mux routeMux) {
	mux.HandleFunc("POST /sites/{name}/cloud/connect", s.auth(s.cloudConnect))
	mux.HandleFunc("POST /sites/{name}/cloud/configure", s.auth(s.cloudConfigure))
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
	if v.Kind == "" && v.Connected {
		v.Kind = "gdrive" // a helper from before the other storage
	}
	if v.Where == "" && v.Kind == "gdrive" {
		v.Where = "Google Drive: " + cloudprov.RootName
	}
	if !v.Connected && v.Pending == nil {
		v.Forms = cloudForms()
	}
	if p, ok := cloudprov.Find(v.Kind); ok && p.Generated {
		v.KindName = providerName(p)
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

// cloudRefusals turn the markers mpn prints into a sentence for the person.
var cloudRefusals = []struct{ marker, key string }{
	{"cloud-not-connected", "cld.err.notconnected"}, {"cloud-connected", "cld.err.connected"}, {"cloud-pending", "cld.err.pending"},
	{"cloud-denied", "cld.err.denied"}, {"cloud-expired", "cld.err.expired"}, {"cloud-busy", "cld.err.busy"},
	{"Google refused the client", "cld.err.client"}, {"cloud-exists", "cld.err.exists"},
	{"cloud-test-connect", "cld.err.test.connect"}, {"cloud-test-write", "cld.err.test.write"}, {"cloud-test-read", "cld.err.test.read"},
	{"cloud-test-delete", "cld.err.test.delete"}, {"cloud-badsettings", "cld.err.settings"},
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
	res := s.runText(r, T, T(title, row.Name), req)
	res.Back = back
	if !res.OK {
		for _, x := range cloudRefusals {
			if strings.Contains(res.Output, x.marker) {
				res.Error = T(x.key)
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

// cloudConfigure connects S3, B2, SFTP, WebDAV or FTP. Only the fields of the chosen type are read from the form, so a field added to
// the request by hand never reaches the helper; the helper checks the same list again.
func (s *Server) cloudConfigure(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.cloudAction(w, r, ss, "act.cld.configure", func(string) (ctl.Request, string) {
		p, ok := cloudprov.Find(r.PostFormValue("type"))
		if !ok {
			return ctl.Request{}, T("cld.err.settings")
		}
		in := cloudprov.Settings{Type: p.Type, Values: map[string]string{}, Folder: strings.Trim(strings.TrimSpace(r.PostFormValue("folder")), "/")}
		if p.Bucket {
			in.Bucket = strings.TrimSpace(r.PostFormValue("bucket"))
		}
		for _, f := range p.Fields {
			if v := r.PostFormValue("v_" + f.Key); v != "" {
				in.Values[f.Key] = v
			}
		}
		if _, err := in.Check(); err != nil {
			return ctl.Request{}, T("cld.err.settings.d", err.Error())
		}
		return ctl.Request{Op: "cloud.configure", CloudStorage: &in}, ""
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
