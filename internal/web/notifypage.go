package web

import (
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mpn/internal/ctl"
	"mpn/internal/notify"
	"mpn/internal/webauth"
)

// notifyView is the page of the Telegram alerts.
type notifyView struct {
	S         notify.Status
	Nodes     []notifyNodeRow
	Code      string // a pairing code just made, shown once
	CodeFor   string
	HubURL    string // the address agents reach this panel at, as a guess for the form
	EditNode  *notifyNodeRow
	AgentLast string // when the agent last reached the hub
	Error     string
}

type notifyNodeRow struct {
	notify.NodeView
	Seen  string
	State string // ok, alert, lost, new
	Rules notify.Rules
}

func (s *Server) notifyPages(mux routeMux) {
	mux.HandleFunc("GET /notify", s.auth(s.notifyPage))
	for _, a := range []string{"hub", "chat", "test", "rules", "pair", "agent", "off", "noderules", "noderemove"} {
		mux.HandleFunc("POST /notify/"+a, s.auth(s.notifyAction(a)))
	}
}

func (s *Server) notifyStatus(T translator) (notify.Status, string) {
	var st notify.Status
	if e := s.listText(T, ctl.Request{Op: "notify.status"}, &st); e != "" {
		return st, e
	}
	return st, ""
}

func (s *Server) notifyPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	T := translatorForWriter(w)
	st, e := s.notifyStatus(T)
	v := &notifyView{S: st, Error: e, HubURL: guessHubURL(r)}
	now := time.Now()
	if st.Agent.LastOK > 0 {
		v.AgentLast = time.Unix(st.Agent.LastOK, 0).Format("02/01/2006 15:04:05")
	}
	for _, n := range st.Nodes {
		row := notifyNodeRow{NodeView: n, Rules: st.Rules}
		if n.OwnRules != nil {
			row.Rules = *n.OwnRules
		}
		switch {
		case n.Report.Version == "":
			row.State = "new"
		case n.Lost || now.Sub(time.Unix(n.LastSeen, 0)) >= notify.LostAfter:
			row.State = "lost"
		case len(n.Report.Alerts) > 0:
			row.State = "alert"
		default:
			row.State = "ok"
		}
		if n.LastSeen > 0 {
			row.Seen = notify.Duration(now.Sub(time.Unix(n.LastSeen, 0)))
		}
		v.Nodes = append(v.Nodes, row)
	}
	sort.SliceStable(v.Nodes, func(i, j int) bool { return v.Nodes[i].Name < v.Nodes[j].Name })
	if id, _ := strconv.ParseInt(r.URL.Query().Get("node"), 10, 64); id > 0 {
		for i := range v.Nodes {
			if v.Nodes[i].ID == id {
				v.EditNode = &v.Nodes[i]
			}
		}
	}
	if code, name := s.takePairCode(ss.CSRF); code != "" {
		v.Code, v.CodeFor = code, name
	}
	s.show(w, 200, ss, view{Page: "notify", Notify: v})
}

// guessHubURL is the address the browser used to reach this panel, which is usually what the other machines can reach too.
func guessHubURL(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil && (h == "localhost" || h == "127.0.0.1" || h == "::1") {
		return ""
	}
	return "https://" + host
}

// rulesFromForm reads the rules of the form.
func rulesFromForm(r *http.Request) notify.Rules {
	n := func(k string) int { v, _ := strconv.Atoi(strings.TrimSpace(r.PostFormValue(k))); return v }
	on := func(k string) bool { return r.PostFormValue(k) == "on" }
	return notify.Rules{CPU: n("cpu"), CPUMin: n("cpu_min"), Mem: n("mem"), MemMin: n("mem_min"), Disk: n("disk"), CertDays: n("cert_days"),
		Repeat: n("repeat"), Resources: on("resources"), Services: on("services"), Sites: on("sites"), Security: on("security"), Ops: on("ops"), Danger: on("danger")}
}

func (s *Server) notifyAction(action string) func(http.ResponseWriter, *http.Request, webauth.Session) {
	return func(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
		T := translatorForWriter(w)
		back := "/notify"
		var req ctl.Request
		switch action {
		case "hub":
			req = ctl.Request{Op: "notify.hub", Name: strings.TrimSpace(r.PostFormValue("name")), Lang: r.PostFormValue("lang"), Token: strings.TrimSpace(r.PostFormValue("token"))}
		case "chat":
			id, _ := strconv.ParseInt(r.PostFormValue("chat"), 10, 64)
			req = ctl.Request{Op: "notify.chat", ChatID: id}
		case "test":
			req = ctl.Request{Op: "notify.test"}
		case "rules":
			rules := rulesFromForm(r)
			req = ctl.Request{Op: "notify.rules", Rules: &rules, Lang: r.PostFormValue("lang")}
		case "noderules":
			id, _ := strconv.ParseInt(r.PostFormValue("node"), 10, 64)
			req = ctl.Request{Op: "notify.noderules", NodeID: id}
			if r.PostFormValue("reset") == "" {
				rules := rulesFromForm(r)
				req.Rules = &rules
			}
		case "noderemove":
			id, _ := strconv.ParseInt(r.PostFormValue("node"), 10, 64)
			req = ctl.Request{Op: "notify.noderemove", NodeID: id}
		case "pair":
			name := strings.TrimSpace(r.PostFormValue("name"))
			res := s.runText(r, T, T("ntf.act.pair", name), ctl.Request{Op: "notify.pair", Name: name, HubURL: strings.TrimSpace(r.PostFormValue("url"))})
			if res.OK {
				// The code is shown once on the page, not on the page of the result, and never in an address.
				s.putPairCode(ss.CSRF, strings.TrimSpace(res.Output), name)
				http.Redirect(w, r, back, http.StatusSeeOther)
				return
			}
			res.Back = back
			s.finish(w, r, res)
			return
		case "agent":
			req = ctl.Request{Op: "notify.agent", Name: strings.TrimSpace(r.PostFormValue("name")), Code: r.PostFormValue("code")}
		case "off":
			if r.PostFormValue("confirm") != "off" {
				s.refuseTo(w, ss, back, T("err.nothing"), T("err.confirm"))
				return
			}
			req = ctl.Request{Op: "notify.off"}
		}
		res := s.runText(r, T, T("ntf.act."+action), req)
		res.Back = back
		s.finish(w, r, res)
	}
}

// The pairing code of a session, kept for one look.
func (s *Server) putPairCode(session, code, name string) {
	s.flashMu.Lock()
	defer s.flashMu.Unlock()
	if s.pairCodes == nil {
		s.pairCodes = map[string][2]string{}
	}
	s.pairCodes[session] = [2]string{code, name}
}

func (s *Server) takePairCode(session string) (string, string) {
	s.flashMu.Lock()
	defer s.flashMu.Unlock()
	c, ok := s.pairCodes[session]
	if !ok {
		return "", ""
	}
	delete(s.pairCodes, session)
	return c[0], c[1]
}
