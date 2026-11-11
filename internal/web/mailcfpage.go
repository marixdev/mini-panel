package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"mpn/internal/cfdns"
	"mpn/internal/ctl"
	"mpn/internal/mail"
	"mpn/internal/webauth"
)

// Adding the DNS records of a mail domain to Cloudflare. The button shows only for a domain whose name servers are Cloudflare's. The
// person brings an API token made for that zone; it goes to the root helper on standard input, is used for one request and is not kept.

const cfSeenFor = 10 * time.Minute

type cfSeen struct {
	yes bool
	at  time.Time
}

// cfCache remembers what the name servers of a domain said, so that opening the Email page does not ask the Internet each time.
type cfCache struct {
	mu sync.Mutex
	m  map[string]cfSeen
}

func (s *Server) cloudflareOf(domain string) bool {
	s.cf.mu.Lock()
	e, ok := s.cf.m[domain]
	s.cf.mu.Unlock()
	if ok && time.Since(e.at) < cfSeenFor {
		return e.yes
	}
	var d struct {
		Cloudflare bool `json:"cloudflare"`
	}
	if msg := s.list(ctl.Request{Op: "mail.cfdetect", MailDomain: domain}, &d); msg != "" {
		return false // a lookup that failed is not remembered: the next visit asks again
	}
	s.cf.mu.Lock()
	if s.cf.m == nil || len(s.cf.m) > 200 {
		s.cf.m = map[string]cfSeen{}
	}
	s.cf.m[domain] = cfSeen{d.Cloudflare, time.Now()}
	s.cf.mu.Unlock()
	return d.Cloudflare
}

// markCloudflare asks about every domain at once.
func (s *Server) markCloudflare(v *mailView) {
	if !v.On {
		return
	}
	var wg sync.WaitGroup
	for i := range v.Domains {
		wg.Add(1)
		go func(d *mailDomainRow) {
			defer wg.Done()
			d.Cloudflare = s.cloudflareOf(d.Name)
		}(&v.Domains[i])
	}
	wg.Wait()
}

// cfView is what the page that asks for the token draws.
type cfView struct {
	Domain   string
	Host     string
	ServerIP string
	Records  []mail.Record
	Error    string
}

func (s *Server) mailCFPages(mux *http.ServeMux) {
	mux.HandleFunc("GET /email/cf", s.auth(s.cfPage))
	mux.HandleFunc("POST /email/cf", s.auth(s.cfAdd))
}

// cfFor finds the domain among those the mail server has. Nothing else is accepted, so the name in the address is never passed on
// unchecked.
func (s *Server) cfFor(domain string, r *http.Request) (*cfView, string) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if err := mail.ValidDomain(domain); err != nil {
		return nil, T("mail.err.domain")
	}
	st := &mailView{}
	if msg := s.list(ctl.Request{Op: "mail.status"}, st); msg != "" {
		return nil, msg
	}
	for _, d := range st.Domains {
		if d.Name == domain {
			return &cfView{Domain: domain, Host: st.Host, ServerIP: serverIP(r.Host), Records: d.Records}, ""
		}
	}
	return nil, T("mail.cf.err.nodomain")
}

func (s *Server) cfPage(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v, msg := s.cfFor(r.URL.Query().Get("domain"), r)
	if v == nil {
		s.mailRefuse(w, r, ss, msg)
		return
	}
	s.show(w, http.StatusOK, ss, view{Page: "emailcf", CF: v})
}

// cfReport is what "mpn mail cf-add -json" prints.
type cfReport struct {
	Zone    string `json:"zone"`
	HostIP  string `json:"host_ip"`
	Results []struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Note   string `json:"note"`
	} `json:"results"`
}

// cfLines says the report in words, one record to a line, and whether any record met a different one that was left alone.
func cfLines(rep cfReport) (text string, conflict bool) {
	var b strings.Builder
	for _, r := range rep.Results {
		b.WriteString(T("mail.cf.st."+r.Status) + "  " + r.Type + "  " + r.Name)
		if r.Note != "" {
			b.WriteString("  (" + r.Note + ")")
		}
		b.WriteString("\n")
		conflict = conflict || r.Status == "conflict"
	}
	return strings.TrimRight(b.String(), "\n"), conflict
}

func (s *Server) cfAdd(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	v, msg := s.cfFor(r.PostFormValue("domain"), r)
	if v == nil {
		s.mailRefuse(w, r, ss, msg)
		return
	}
	token := strings.TrimSpace(r.PostFormValue("token"))
	if !cfdns.ValidToken(token) {
		v.Error = T("mail.cf.err.token")
		s.show(w, http.StatusBadRequest, ss, view{Page: "emailcf", CF: v}) // the token is not put back in the form
		return
	}
	res := s.run(T("act.mail.cf", v.Domain), ctl.Request{Op: "mail.cfadd", MailDomain: v.Domain, CFToken: token})
	res.Back = "/email"
	if res.OK {
		var rep cfReport
		if json.Unmarshal([]byte(res.Output), &rep) == nil {
			text, conflict := cfLines(rep)
			res.Output = T("mail.cf.zone", rep.Zone) + "\n\n" + text
			if rep.HostIP == "" {
				res.Output += "\n\n" + T("mail.cf.nohostip", v.Host)
			}
			if conflict {
				res.Output += "\n\n" + T("mail.cf.conflict")
			}
		}
	} else {
		all := res.Output + " " + res.Error
		for marker, key := range map[string]string{"cf-badtoken": "mail.cf.err.badtoken", "cf-noperm": "mail.cf.err.noperm", "cf-nozone": "mail.cf.err.nozone",
			"cf-ratelimit": "mail.cf.err.ratelimit", "cf-network": "mail.cf.err.network", "mail-off": "mail.cf.err.off"} {
			if strings.Contains(all, marker) {
				res.Error = T(key)
				break
			}
		}
		res.Output = "" // what the helper printed is for the command line, and could name a record
	}
	s.finish(w, r, res)
}
