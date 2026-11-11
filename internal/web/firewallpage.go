package web

import (
	"net/http"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/firewall"
	"mpn/internal/webauth"
)

// fwView is what the firewall page draws; the first fields are the answer of "mpn firewall status".
type fwView struct {
	On       bool            `json:"on"`
	Active   bool            `json:"active"`
	Conflict string          `json:"conflict"`
	HasNft   bool            `json:"has_nft"`
	Locked   firewall.Locked `json:"locked"`
	Rules    []firewall.Rule `json:"rules"`
	Error    string          `json:"-"`
	Client   string          `json:"-"`
	Max      int             `json:"-"`
}

func (s *Server) firewallPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /firewall", s.auth(s.firewallPage))
	mux.HandleFunc("POST /firewall/enable", s.auth(s.fwEnable))
	mux.HandleFunc("POST /firewall/disable", s.auth(s.fwDisable))
	mux.HandleFunc("POST /firewall/add", s.auth(s.fwAdd))
	mux.HandleFunc("POST /firewall/block", s.auth(s.fwBlock))
	mux.HandleFunc("POST /firewall/{id}/delete", s.auth(s.fwDelete))
}

func (s *Server) firewallPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	v := &fwView{Client: s.clientKey(r), Max: firewall.MaxRules}
	v.Error = s.listText(T, ctl.Request{Op: "firewall.status"}, v)
	s.show(w, 200, ss, view{Page: "firewall", FW: v})
}

// fwAction runs one operation and shows the result, with the sentences that say what a refusal means.
func (s *Server) fwAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, build func() (ctl.Request, string)) {
	T := translatorForWriter(w)
	req, problem := build()
	if problem != "" {
		s.refuseTo(w, ss, "/firewall", T("fw.fail"), problem)
		return
	}
	res := s.runText(T, T(title), req)
	res.Back = "/firewall"
	if !res.OK {
		switch {
		case strings.Contains(res.Output, "firewall-conflict"):
			res.Error = T("fw.err.conflict")
		case strings.Contains(res.Output, "would block"):
			res.Error = T("fw.err.lockout")
		case strings.Contains(res.Output, "already taken"):
			res.Error = T("fw.err.same")
		}
	}
	s.finish(w, r, res)
}

func (s *Server) fwEnable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.fwAction(w, r, ss, "act.fw.enable", func() (ctl.Request, string) { return ctl.Request{Op: "firewall.enable"}, "" })
}

func (s *Server) fwDisable(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.fwAction(w, r, ss, "act.fw.disable", func() (ctl.Request, string) { return ctl.Request{Op: "firewall.disable"}, "" })
}

func (s *Server) fwAdd(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.fwAction(w, r, ss, "act.fw.add", func() (ctl.Request, string) {
		rule := firewall.Rule{Action: r.PostFormValue("action"), Proto: r.PostFormValue("proto"), Port: strings.TrimSpace(r.PostFormValue("port")),
			Source: strings.TrimSpace(r.PostFormValue("source")), Comment: strings.TrimSpace(r.PostFormValue("comment"))}
		if _, err := firewall.ValidRule(rule); err != nil {
			return ctl.Request{}, T("fw.err.rule", err.Error())
		}
		return ctl.Request{Op: "firewall.add", FwAction: rule.Action, FwProto: rule.Proto, FwPort: rule.Port, FwSource: rule.Source, FwComment: rule.Comment, Client: s.clientKey(r)}, ""
	})
}

// fwBlock is the short form most people want: stop one address or one network from reaching the server at all.
func (s *Server) fwBlock(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.fwAction(w, r, ss, "act.fw.block", func() (ctl.Request, string) {
		src := strings.TrimSpace(r.PostFormValue("source"))
		if src == "" {
			return ctl.Request{}, T("fw.err.nosource")
		}
		rule := firewall.Rule{Action: firewall.Deny, Proto: "both", Source: src, Comment: strings.TrimSpace(r.PostFormValue("comment"))}
		if _, err := firewall.ValidRule(rule); err != nil {
			return ctl.Request{}, T("fw.err.rule", err.Error())
		}
		return ctl.Request{Op: "firewall.add", FwAction: rule.Action, FwProto: rule.Proto, FwSource: rule.Source, FwComment: rule.Comment, Client: s.clientKey(r)}, ""
	})
}

func (s *Server) fwDelete(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	s.fwAction(w, r, ss, "act.fw.del", func() (ctl.Request, string) {
		id, ok := jobID(r)
		if !ok {
			return ctl.Request{}, T("fw.err.id")
		}
		return ctl.Request{Op: "firewall.del", RuleID: id}, ""
	})
}
