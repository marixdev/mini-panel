package ctl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"mpn/internal/audit"
)

// Audited says whether an operation goes into the activity log: everything that changes the machine, and the opening of the
// database browser, which hands out a password. Reading pages is not logged.
func Audited(op string) bool {
	switch op {
	case "db.pmadrop", "db.pmaclean": // the panel tidies up after the database browser by itself
		return false
	case "db.pma":
		return true
	}
	return IsWrite(op)
}

// AuditDetail describes a request for the log: the names it acts on and the choices it makes, never a password, a key, a
// certificate, a token, a command line or a configuration. A field that is not on the list below is not written at all, so a new
// field stays out of the log until somebody decides it is safe.
func (r Request) AuditDetail() string {
	var p []string
	add := func(k, v string) {
		if v != "" {
			p = append(p, k+"="+v)
		}
	}
	yes := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	add("name", r.Name)
	add("kind", r.Kind)
	add("user", r.User)
	if len(r.Hosts) > 0 {
		add("hosts", strings.Join(r.Hosts, ","))
	}
	add("php", r.PHP)
	add("node", r.Node)
	add("python", r.Python)
	add("domain", r.Domain)
	add("www", r.WWW)
	add("backup", r.Backup)
	if len(r.Databases) > 0 {
		add("databases", strings.Join(r.Databases, ","))
	}
	if r.Keep > 0 {
		add("keep", strconv.Itoa(r.Keep))
	}
	if r.JobID > 0 {
		add("job", strconv.FormatInt(r.JobID, 10))
	}
	if r.RuleID > 0 {
		add("rule", strconv.FormatInt(r.RuleID, 10))
	}
	add("action", r.FwAction)
	add("proto", r.FwProto)
	add("port", r.FwPort)
	add("source", r.FwSource)
	add("sec_mode", r.SecMode)
	add("jail", r.Jail)
	add("ip", r.IP)
	add("app", r.App)
	add("wp", strings.Trim(r.WPKind+" "+r.WPAction+" "+r.WPSlug, " "))
	add("mail_host", r.MailHost)
	add("mail_domain", r.MailDomain)
	add("mail_address", r.MailAddress)
	add("svc", strings.Trim(r.SvcKey+" "+r.SvcAction, " "))
	if strings.HasPrefix(r.Op, "port.") && r.Port > 0 {
		add("port", strconv.Itoa(r.Port))
	}
	add("db_series", r.DBSeries)
	add("version", r.Version)
	if strings.HasPrefix(r.Op, "notify.") {
		if r.ChatID != 0 {
			add("chat", strconv.FormatInt(r.ChatID, 10))
		}
		if r.NodeID != 0 {
			add("node", strconv.FormatInt(r.NodeID, 10))
		}
		add("url", r.HubURL)
		add("lang", r.Lang)
	}
	if r.CloudStorage != nil {
		add("storage", r.CloudStorage.Type)
	}
	// The yes/no choices mean something only for some operations.
	switch r.Op {
	case "site.remove", "mail.boxdel", "mail.domaindel":
		add("purge", yes(r.Purge))
	case "site.access", "site.webmail", "mail.webmail":
		add("on", yes(r.Enable))
	case "site.cronset":
		add("on", yes(r.On))
	case "site.sslredirect":
		add("on", yes(r.Redirect))
	case "site.speed":
		add("on", yes(r.Speed))
	case "security.f2b", "security.cloudflare":
		add("on", yes(r.SecOn))
	case "cloud.enable":
		add("on", yes(r.CloudOn))
	case "backup.restore":
		if r.All {
			add("all", "yes")
		}
		if r.Files {
			add("files", "yes")
		}
	case "site.set":
		if len(r.PHPIni) > 0 {
			keys := make([]string, 0, len(r.PHPIni))
			for k := range r.PHPIni {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			add("php_ini", strings.Join(keys, ","))
		}
		if r.Mem > 0 {
			add("mem", fmt.Sprintf("%dMB", r.Mem))
		}
	}
	return audit.CleanDetail(strings.Join(p, " "))
}

// Dangerous says whether an operation is one the owner of the server wants to hear about at once: something deleted, a protection
// switched off, a password changed, a backup put back over the live site, the program replaced.
func (r Request) Dangerous() bool {
	switch r.Op {
	case "site.remove", "db.remove", "db.passwd", "backup.restore", "firewall.disable", "firewall.del", "mail.disable",
		"mail.domaindel", "mail.boxdel", "mail.boxpasswd", "site.ssloff", "panel.ssloff", "panel.update", "cloud.disconnect",
		"site.vhostset", "site.panelkey", "services.remove", "notify.off", "notify.noderemove", "notify.hub", "notify.agent":
		return true
	case "security.waf":
		return r.SecMode != "block"
	case "security.f2b":
		return !r.SecOn
	case "site.access":
		return r.Enable // a password for logging in as the site's account
	case "db.remote":
		return len(r.Hosts) > 0 // a database opened to other machines
	case "services.control":
		return r.SvcAction == "stop"
	case "port.set", "port.finish":
		return r.SvcKey == "ssh" || r.SvcKey == "panel" // the way into the machine moves
	}
	return false
}

// DangerousEvent is Dangerous for the events that only the web side sees.
func DangerousEvent(op string) bool {
	switch op {
	case "account.password", "account.2fa_off", "account.setup":
		return true
	}
	return false
}

// validNote checks an event the web side sends for the log.
func (r Request) validNote() error {
	if !audit.Events[r.Event] {
		return fmt.Errorf("unknown event %q", r.Event)
	}
	if err := optionalSite(r.Site); err != nil {
		return err
	}
	if len(r.Note) > 2*audit.MaxDetail {
		return fmt.Errorf("the note is too long")
	}
	return nil
}

// NoteEntry is the log entry of an audit.note request.
func (r Request) NoteEntry() audit.Entry {
	return audit.Entry{IP: r.Actor, Site: r.Site, Op: r.Event, Kind: audit.Category(r.Event), Detail: audit.CleanDetail(r.Note),
		OK: !r.Failed, Danger: !r.Failed && DangerousEvent(r.Event)}
}

// LogEntry is the log entry of a request the helper ran, with its outcome.
func (r Request) LogEntry(err error) audit.Entry {
	e := audit.Entry{IP: r.Actor, Site: r.Site, Op: r.Op, Kind: audit.Category(r.Op), Detail: r.AuditDetail(), OK: err == nil}
	if err != nil {
		e.Error = audit.CleanDetail(err.Error())
	}
	e.Danger = e.OK && r.Dangerous()
	return e
}

// auditList reads the activity log with mpn audit list.
func (r Request) auditList() ([]string, error) {
	if err := optionalSite(r.Site); err != nil {
		return nil, err
	}
	if err := audit.ValidIPFilter(r.AuditIP); err != nil {
		return nil, err
	}
	if err := audit.ValidKind(r.AuditKind); err != nil {
		return nil, err
	}
	if err := audit.ValidDay(r.From); err != nil {
		return nil, err
	}
	if err := audit.ValidDay(r.To); err != nil {
		return nil, err
	}
	if r.Limit < 0 || r.Limit > audit.MaxPage || r.Offset < 0 || r.Offset > 10_000_000 {
		return nil, fmt.Errorf("the page of the log is out of range")
	}
	// "=" joins each value to its flag, so an empty value cannot be read as the next flag.
	return []string{"audit", "list", "-json", "-site=" + r.Site, "-ip=" + r.AuditIP, "-kind=" + r.AuditKind, "-from=" + r.From, "-to=" + r.To,
		"-limit=" + strconv.Itoa(r.Limit), "-offset=" + strconv.Itoa(r.Offset)}, nil
}
