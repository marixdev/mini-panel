package web

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"mpn/internal/ctl"
	"mpn/internal/firewall"
	"mpn/internal/svcport"
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
	Ports    *svcport.Status `json:"-"` // the ports of the services; nil when they could not be read
	PortErr  string          `json:"-"`
	Host     string          `json:"-"` // the name or address the page was opened on, without a port
	OnPort   int             `json:"-"` // the port the page was opened on
}

// SSHNew are the ports a waiting change of SSH adds.
func (v *fwView) SSHNew() []int {
	if v.Ports == nil {
		return nil
	}
	return v.Ports.SSHNew()
}

func (s *Server) firewallPages(mux routeMux) {
	mux.HandleFunc("GET /firewall", s.auth(s.firewallPage))
	mux.HandleFunc("POST /firewall/enable", s.auth(s.fwEnable))
	mux.HandleFunc("POST /firewall/disable", s.auth(s.fwDisable))
	mux.HandleFunc("POST /firewall/add", s.auth(s.fwAdd))
	mux.HandleFunc("POST /firewall/block", s.auth(s.fwBlock))
	mux.HandleFunc("POST /firewall/{id}/delete", s.auth(s.fwDelete))
	mux.HandleFunc("POST /firewall/port", s.auth(s.fwPortSet))
	mux.HandleFunc("POST /firewall/port/finish", s.auth(s.fwPortEnd("finish")))
	mux.HandleFunc("POST /firewall/port/cancel", s.auth(s.fwPortEnd("cancel")))
}

func (s *Server) firewallPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	v := &fwView{Client: s.clientAddr(r), Max: firewall.MaxRules, Host: hostOnly(r.Host), OnPort: requestPort(r)}
	v.Error = s.listText(T, ctl.Request{Op: "firewall.status"}, v)
	var p svcport.Status
	if e := s.listText(T, ctl.Request{Op: "port.status"}, &p); e != "" {
		v.PortErr = e
	} else {
		v.Ports = &p
	}
	s.show(w, 200, ss, view{Page: "firewall", FW: v})
}

// requestPort is the port the browser used for this request: the one in the Host header, or 443.
func requestPort(r *http.Request) int {
	if _, p, err := net.SplitHostPort(r.Host); err == nil {
		n, _ := strconv.Atoi(p)
		return n
	}
	return 443
}

// fwPortSet changes the port of one service. SSH and the panel then wait for the person to finish the change from the new port.
func (s *Server) fwPortSet(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	svc := r.PostFormValue("service")
	port, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("port")))
	if err != nil {
		port = 0
	}
	if err := svcport.Valid(svc, port); err != nil {
		s.refuseTo(w, ss, "/firewall", T("port.fail"), T("port.err.bad", err.Error()))
		return
	}
	res := s.runText(r, T, T("act.port.set", T("port.svc."+svc), port), ctl.Request{Op: "port.set", SvcKey: svc, Port: port})
	res.Back = "/firewall"
	if !res.OK {
		if strings.Contains(res.Output+res.Error, "port-waiting") {
			res.Error = T("port.err.waiting")
		}
		s.finish(w, r, res)
		return
	}
	if svc == svcport.Panel {
		// The next step is taken from the new port, which proves it works. The page there is the Firewall page of the same panel; a
		// cookie does not depend on the port, so the session goes with it.
		res.Back = "https://" + net.JoinHostPort(hostOnly(r.Host), strconv.Itoa(port)) + "/firewall"
	}
	s.finish(w, r, res)
}

func (s *Server) fwPortEnd(step string) func(http.ResponseWriter, *http.Request, webauth.Session) {
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		T := translatorForWriter(w)
		svc := r.PostFormValue("service")
		if !svcport.TwoStep(svc) {
			s.refuseTo(w, ss, "/firewall", T("port.fail"), T("port.err.svc"))
			return
		}
		res := s.runText(r, T, T("act.port."+step, T("port.svc."+svc)), ctl.Request{Op: "port." + step, SvcKey: svc})
		res.Back = "/firewall"
		s.finish(w, r, res)
	}
}

// fwAction runs one operation and shows the result, with the sentences that say what a refusal means.
func (s *Server) fwAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, build func() (ctl.Request, string)) {
	T := translatorForWriter(w)
	req, problem := build()
	if problem != "" {
		s.refuseTo(w, ss, "/firewall", T("fw.fail"), problem)
		return
	}
	res := s.runText(r, T, T(title), req)
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
		return ctl.Request{Op: "firewall.add", FwAction: rule.Action, FwProto: rule.Proto, FwPort: rule.Port, FwSource: rule.Source, FwComment: rule.Comment, Client: s.clientAddr(r)}, ""
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
		return ctl.Request{Op: "firewall.add", FwAction: rule.Action, FwProto: rule.Proto, FwSource: rule.Source, FwComment: rule.Comment, Client: s.clientAddr(r)}, ""
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
