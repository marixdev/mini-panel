package web

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// siteInfo is the answer of "mpn site info": what the overview of a site shows.
type siteInfo struct {
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	User          string            `json:"user"`
	Hostnames     []string          `json:"hostnames"`
	Root          string            `json:"root"`
	RootSub       string            `json:"root_sub"`
	RootOutside   bool              `json:"root_outside"`
	TLS           bool              `json:"tls"`
	CertExpiry    int64             `json:"cert_expiry"`
	Running       bool              `json:"running"`
	RunDetail     string            `json:"run_detail"`
	DiskBytes     int64             `json:"disk_bytes"`
	DiskCut       bool              `json:"disk_cut"`
	LastBackup    string            `json:"last_backup"`
	PHPVersion    string            `json:"php_version"`
	PHPValues     map[string]string `json:"php_values"`
	PHPSet        map[string]string `json:"php_set"`
	PHPMemory     int               `json:"php_memory_default"`
	NodeVer       string            `json:"node_version"`
	PyVer         string            `json:"python_version"`
	Entry         string            `json:"entry"`
	Port          int               `json:"port"`
	MemMB         int               `json:"mem_mb"`
	Upstream      string            `json:"upstream"`
	InstalledPHP  []string          `json:"installed_php"`
	InstalledNode []string          `json:"installed_node"`
	InstalledPy   []string          `json:"installed_python"`
	Nginx         string            `json:"nginx"`
}

// phpField is one PHP setting as the form shows it.
type phpField struct {
	Name, Value, Default, Hint string
	Common, Changed            bool
	Choices                    []string
}

// overview is what the overview tab draws, worked out from the answer of the helper.
type overview struct {
	siteInfo
	DiskText, CertText, BackupText string
	CertSoon                       bool // fewer than 14 days left
	PHPVersions, NodeVersions      []string
	PyVersions                     []string
	Fields                         []phpField
	More                           []phpField // settings that are not one of the six common ones
	Home                           string     // the folder every web root lies in
}

func humanBytes(n int64) string { return humanSize(n) }

func (s *Server) loadOverview(row siteRow) (*overview, string) {
	var in siteInfo
	if e := s.list(ctl.Request{Op: "site.info", Site: row.Name}, &in); e != "" {
		return nil, e
	}
	o := &overview{siteInfo: in, Home: site.HtdocsOf(row.User) + "/"}
	if in.DiskCut {
		o.DiskText = T("ov.disk.big")
	} else {
		o.DiskText = humanBytes(in.DiskBytes)
	}
	if in.CertExpiry > 0 {
		exp := time.Unix(in.CertExpiry, 0)
		days := int(time.Until(exp).Hours() / 24)
		o.CertText, o.CertSoon = T("ov.cert.until", exp.Local().Format("02/01/2006"), days), days < 14
	}
	if in.LastBackup != "" {
		o.BackupText = stampText(in.LastBackup)
	}
	// Only versions that are installed are offered, and the one in use is always among them.
	o.PHPVersions = withCurrent(in.InstalledPHP, in.PHPVersion)
	o.NodeVersions = withCurrent(in.InstalledNode, in.NodeVer)
	o.PyVersions = withCurrent(in.InstalledPy, in.PyVer)
	if row.Kind == string(site.PHP) {
		for _, d := range site.PHPSettings(strconv.Itoa(in.PHPMemory) + "M") {
			f := phpField{Name: d.Name, Default: d.Default, Hint: hintFor(d.Name, d.Hint), Common: d.Common, Choices: d.Choices, Value: in.PHPValues[d.Name]}
			_, f.Changed = in.PHPSet[d.Name]
			if f.Common {
				o.Fields = append(o.Fields, f)
			} else {
				o.More = append(o.More, f)
			}
		}
	}
	return o, ""
}

// hintFor is the Vietnamese description of a PHP setting; the English one of the site package is the fallback.
func hintFor(name, fallback string) string {
	if s, ok := vi["ph."+name]; ok {
		return s
	}
	return fallback
}

func withCurrent(installed []string, cur string) []string {
	out := slices.Clone(installed)
	if cur != "" && !slices.Contains(out, cur) {
		out = append(out, cur)
	}
	return out
}

// settingsRequest reads the settings form of a site into a request for the helper. The kind comes from the stored site, never
// from the form. Every value is checked here too, so that a mistake is explained in Vietnamese next to its name.
func (s *Server) settingsRequest(r *http.Request, row siteRow, info siteInfo) (ctl.Request, string) {
	req := ctl.Request{Op: "site.set", Site: row.Name, Kind: row.Kind}
	get := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	kind := site.Kind(row.Kind)
	if kind != site.Proxy {
		if info.RootOutside {
			return req, T("set.err.rootoutside")
		}
		req.RootSub = strings.Trim(get("root_sub"), "/")
		if !site.ValidSubdir(req.RootSub) {
			return req, T("set.err.root")
		}
	}
	switch kind {
	case site.PHP:
		req.PHP = get("php")
		if !site.ValidPHPVersion(req.PHP) || !slices.Contains(withCurrent(info.InstalledPHP, info.PHPVersion), req.PHP) {
			return req, T("set.err.php", req.PHP)
		}
		req.PHPIni = map[string]string{}
		for _, d := range site.PHPSettings(strconv.Itoa(info.PHPMemory) + "M") {
			v := get("ini_" + d.Name)
			if v == "" || v == d.Default {
				continue // what the machine does anyway is not stored
			}
			if hint, ok := site.CheckPHPSetting(d.Name, v); !ok {
				return req, T("set.err.ini", d.Name, v, hintFor(d.Name, hint))
			}
			req.PHPIni[d.Name] = v
		}
		if site.UploadLargerThanPost(req.PHPIni) {
			return req, T("set.err.upload")
		}
	case site.Node:
		req.Node, req.Entry = get("node"), get("entry")
		if !site.ValidNodeVersion(req.Node) || !slices.Contains(withCurrent(info.InstalledNode, info.NodeVer), req.Node) {
			return req, T("set.err.node", req.Node)
		}
		if !site.ValidEntry(req.Entry) {
			return req, T("set.err.entry")
		}
		var err error
		if req.Port, err = strconv.Atoi(get("port")); err != nil || !site.ValidAppPort(req.Port) {
			return req, T("set.err.port")
		}
		if m := get("mem"); m != "" {
			if req.Mem, err = strconv.Atoi(m); err != nil || !site.ValidMemMB(req.Mem) {
				return req, T("set.err.mem")
			}
		}
	case site.Python:
		req.Python, req.Entry = get("python"), get("entry")
		if !site.ValidPythonVersion(req.Python) || !slices.Contains(withCurrent(info.InstalledPy, info.PyVer), req.Python) {
			return req, T("set.err.python", req.Python)
		}
		if !site.ValidPythonEntry(req.Entry) {
			return req, T("set.err.pyentry")
		}
		var err error
		if req.Port, err = strconv.Atoi(get("port")); err != nil || !site.ValidAppPort(req.Port) {
			return req, T("set.err.port")
		}
		if m := get("mem"); m != "" {
			if req.Mem, err = strconv.Atoi(m); err != nil || !site.ValidMemMB(req.Mem) {
				return req, T("set.err.mem")
			}
		}
	case site.Proxy:
		req.Upstream = get("upstream")
		if !site.ValidUpstream(req.Upstream) {
			return req, T("set.err.upstream")
		}
	}
	return req, ""
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "")
	o, e := s.loadOverview(row)
	if e != "" {
		s.refuseTo(w, ss, back, T("set.fail"), e)
		return
	}
	req, problem := s.settingsRequest(r, row, o.siteInfo)
	if problem != "" {
		s.refuseTo(w, ss, back, T("set.fail"), problem)
		return
	}
	res := s.run(T("act.site.set", row.Name), req)
	res.Back = back
	if !res.OK {
		switch {
		case strings.Contains(res.Output, "is already used by site"):
			res.Error = T("set.err.portused")
		case strings.Contains(res.Output, "is not installed on this machine"):
			res.Error = T("set.err.notinstalled")
		}
		res.Error = fmt.Sprintf("%s %s", res.Error, T("set.restored"))
	}
	s.finish(w, r, res)
}
