package web

import (
	"net/http"
	"strconv"
	"strings"

	"mpn/internal/cron"
	"mpn/internal/ctl"
	"mpn/internal/webauth"
)

// cronPreset is a schedule people ask for often, so they do not have to write five fields.
type cronPreset struct{ Value, Key string }

var cronPresets = []cronPreset{
	{"* * * * *", "cr.p.minute"}, {"*/5 * * * *", "cr.p.5min"}, {"*/15 * * * *", "cr.p.15min"}, {"0 * * * *", "cr.p.hour"},
	{"0 3 * * *", "cr.p.day"}, {"0 3 * * 0", "cr.p.week"}, {"0 3 1 * *", "cr.p.month"},
}

type cronRow struct {
	cron.Job
	Human string
}

// cronView is what the Cron tab draws.
type cronView struct {
	Jobs    []cronRow
	Presets []cronPreset
	Max     int
	Error   string
}

func (s *Server) loadCron(row siteRow) *cronView {
	v := &cronView{Presets: cronPresets, Max: cron.MaxJobs}
	var jobs []cron.Job
	v.Error = s.list(ctl.Request{Op: "site.cronlist", Site: row.Name}, &jobs)
	for _, j := range jobs {
		v.Jobs = append(v.Jobs, cronRow{j, describeSchedule(j.Schedule)})
	}
	return v
}

// describeSchedule says a schedule in words when it is one of the common shapes, and gives it back unchanged otherwise.
func describeSchedule(s string) string {
	switch s {
	case "@hourly":
		return T("cr.d.hour")
	case "@daily", "@midnight":
		return T("cr.d.midnight")
	case "@weekly":
		return T("cr.d.week")
	case "@monthly":
		return T("cr.d.month")
	case "@yearly", "@annually":
		return T("cr.d.year")
	}
	f := strings.Fields(s)
	if len(f) != 5 {
		return s
	}
	min, hour, dom, mon, dow := f[0], f[1], f[2], f[3], f[4]
	num := func(x string) (int, bool) { n, err := strconv.Atoi(x); return n, err == nil }
	switch {
	case min == "*" && hour == "*" && dom == "*" && mon == "*" && dow == "*":
		return T("cr.d.everyminute")
	case strings.HasPrefix(min, "*/") && hour == "*" && dom == "*" && mon == "*" && dow == "*":
		if n, ok := num(min[2:]); ok {
			return T("cr.d.everyn", n)
		}
	case dom == "*" && mon == "*" && dow == "*":
		m, okm := num(min)
		h, okh := num(hour)
		if okm && hour == "*" {
			return T("cr.d.hourly", m)
		}
		if okm && okh {
			return T("cr.d.daily", h, m)
		}
	}
	return s
}

func (s *Server) cronAction(w http.ResponseWriter, r *http.Request, ss webauth.Session, title string, build func(row siteRow) (ctl.Request, string)) {
	row, status, msg := s.findSite(r.PathValue("name"))
	if status != 0 {
		http.Error(w, msg, status)
		return
	}
	back := siteTab(row.Name, "cron")
	req, problem := build(row)
	if problem != "" {
		s.refuseTo(w, ss, back, T("cr.fail"), problem)
		return
	}
	res := s.run(T(title, row.Name), req)
	res.Back = back
	s.finish(w, r, res)
}

func (s *Server) addCron(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.cronAction(w, r, ss, "act.cron.add", func(row siteRow) (ctl.Request, string) {
		schedule := strings.Join(strings.Fields(r.PostFormValue("custom")), " ")
		if schedule == "" {
			schedule = r.PostFormValue("preset")
		}
		command := strings.TrimSpace(strings.ReplaceAll(r.PostFormValue("command"), "\r", ""))
		if err := cron.ValidSchedule(schedule); err != nil {
			return ctl.Request{}, T("cr.err.schedule", err.Error())
		}
		if err := cron.ValidCommand(command); err != nil {
			return ctl.Request{}, T("cr.err.command", err.Error())
		}
		return ctl.Request{Op: "site.cronadd", Site: row.Name, Schedule: schedule, Command: command}, ""
	})
}

func jobID(r *http.Request) (int64, bool) {
	n, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return n, err == nil && n > 0
}

func (s *Server) toggleCron(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.cronAction(w, r, ss, "act.cron.toggle", func(row siteRow) (ctl.Request, string) {
		id, ok := jobID(r)
		if !ok {
			return ctl.Request{}, T("cr.err.id")
		}
		return ctl.Request{Op: "site.cronset", Site: row.Name, JobID: id, On: r.PostFormValue("on") == "1"}, ""
	})
}

func (s *Server) deleteCron(w http.ResponseWriter, r *http.Request, ss webauth.Session) {
	s.cronAction(w, r, ss, "act.cron.del", func(row siteRow) (ctl.Request, string) {
		id, ok := jobID(r)
		if !ok {
			return ctl.Request{}, T("cr.err.id")
		}
		return ctl.Request{Op: "site.crondel", Site: row.Name, JobID: id}, ""
	})
}
