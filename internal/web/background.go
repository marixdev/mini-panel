package web

import (
	"net/http"
	"strconv"
	"strings"

	"mpn/internal/background"
	"mpn/internal/ctl"
	"mpn/internal/site"
	"mpn/internal/webauth"
)

// bgRow is one service as the Background tab draws it.
type bgRow struct {
	background.Service
}

// bgView is what the Background tab draws.
type bgView struct {
	Services []bgRow
	Max      int
	Error    string
	Runtime  string
	User     string
}

// bgRuntime names the runtime a site runs its services with, or "" for a kind that cannot run any.
func bgRuntime(kind string) string {
	switch site.Kind(kind) {
	case site.PHP, site.Node, site.Python:
		return kind
	}
	return ""
}

func (s *Server) loadBgText(T translator, row siteRow) *bgView {
	v := &bgView{Max: background.MaxServices, Runtime: bgRuntime(row.Kind), User: row.User}
	if v.Runtime == "" {
		return v
	}
	var list []background.Service
	v.Error = s.listText(T, ctl.Request{Op: "site.bglist", Site: row.Name}, &list)
	for _, sv := range list {
		v.Services = append(v.Services, bgRow{sv})
	}
	return v
}

// bgAction runs one change of the Background tab. A site that cannot run services is refused here; the helper checks the
// rest again.
func (s *Server) bgAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, build func(row siteRow) (ctl.Request, string)) {
	T := translatorForWriter(w)
	row, status, msg := s.findSiteText(T, r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "background")
	if bgRuntime(row.Kind) == "" {
		s.refuseTo(w, ss, back, T("bg.fail"), T("bg.unsupported"))
		return
	}
	req, problem := build(row)
	if problem != "" {
		s.refuseTo(w, ss, back, T("bg.fail"), problem)
		return
	}
	res := s.runText(r, T, T(title, row.Name), req)
	res.Back = back
	s.finish(w, r, res)
}

// bgForm reads the name, arguments, memory limit and the enabled box of a service form. The values are checked here so a
// message can say what is wrong; the helper checks them again against the stored site.
func bgForm(T translator, r *http.Request) (background.Input, string) {
	in := background.Input{
		Name:    strings.TrimSpace(r.PostFormValue("name")),
		Args:    strings.TrimSpace(strings.ReplaceAll(r.PostFormValue("args"), "\r", "")),
		Enabled: r.PostFormValue("enabled") == "1",
	}
	if m := strings.TrimSpace(r.PostFormValue("memory_mb")); m != "" {
		n, err := strconv.Atoi(m)
		if err != nil {
			return in, T("bg.err.memory", m)
		}
		in.MemoryMB = n
	}
	if err := background.ValidName(in.Name); err != nil {
		return in, T("bg.err.name", err.Error())
	}
	if err := background.ValidMemory(in.MemoryMB); err != nil {
		return in, T("bg.err.memory", err.Error())
	}
	args, err := background.ParseArgs(in.Args)
	if err != nil {
		return in, T("bg.err.args", err.Error())
	}
	if err := background.ValidArgs(site.Python, args); err != nil {
		return in, T("bg.err.args", err.Error())
	}
	return in, ""
}

func (s *Server) addBg(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.bgAction(w, r, ss, "act.bg.add", func(row siteRow) (ctl.Request, string) {
		in, problem := bgForm(T, r)
		if problem != "" {
			return ctl.Request{}, problem
		}
		return ctl.Request{Op: "site.bgadd", Site: row.Name, BgName: in.Name, BgArgs: in.Args, BgEnabled: in.Enabled, BgMemoryMB: in.MemoryMB}, ""
	})
}

func (s *Server) editBg(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.bgAction(w, r, ss, "act.bg.set", func(row siteRow) (ctl.Request, string) {
		id, ok := bgID(r)
		if !ok {
			return ctl.Request{}, T("bg.err.id")
		}
		in, problem := bgForm(T, r)
		if problem != "" {
			return ctl.Request{}, problem
		}
		return ctl.Request{Op: "site.bgset", Site: row.Name, BgID: id, BgName: in.Name, BgArgs: in.Args, BgEnabled: in.Enabled, BgMemoryMB: in.MemoryMB}, ""
	})
}

func bgID(r *http.Request) (int64, bool) {
	n, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return n, err == nil && n > 0
}

// toggleBg turns a service on or off. site.bgset replaces the whole service, so the current name, arguments and memory are
// read from the list first and sent back unchanged; only the enabled state differs.
func (s *Server) toggleBg(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.bgAction(w, r, ss, "act.bg.set", func(row siteRow) (ctl.Request, string) {
		id, ok := bgID(r)
		if !ok {
			return ctl.Request{}, T("bg.err.id")
		}
		var list []background.Service
		if e := s.listText(T, ctl.Request{Op: "site.bglist", Site: row.Name}, &list); e != "" {
			return ctl.Request{}, e
		}
		for _, sv := range list {
			if sv.ID == id {
				return ctl.Request{Op: "site.bgset", Site: row.Name, BgID: id, BgName: sv.Name, BgArgs: sv.Args, BgEnabled: r.PostFormValue("on") == "1", BgMemoryMB: sv.MemoryMB}, ""
			}
		}
		return ctl.Request{}, T("bg.err.id")
	})
}

func (s *Server) restartBg(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.bgAction(w, r, ss, "act.bg.restart", func(row siteRow) (ctl.Request, string) {
		id, ok := bgID(r)
		if !ok {
			return ctl.Request{}, T("bg.err.id")
		}
		return ctl.Request{Op: "site.bgrestart", Site: row.Name, BgID: id}, ""
	})
}

func (s *Server) deleteBg(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.bgAction(w, r, ss, "act.bg.del", func(row siteRow) (ctl.Request, string) {
		id, ok := bgID(r)
		if !ok {
			return ctl.Request{}, T("bg.err.id")
		}
		return ctl.Request{Op: "site.bgdel", Site: row.Name, BgID: id}, ""
	})
}
