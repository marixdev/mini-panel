// Package audit describes the activity log of the panel: who did what, from which address, to which website. The root helper
// writes the log into the mpn database, so the web side, which runs without privileges, can add to it but never change or delete
// what is there. Nothing secret is ever written: no password, key, certificate, token, command line or file content.
package audit

import (
	"errors"
	"net"
	"strings"
	"time"
	"unicode"
)

// Retention is how long an entry is kept.
const Retention = 90 * 24 * time.Hour

// MaxPage is the most entries one request for the log returns.
const MaxPage = 1000

// Entry is one line of the log. Op is an operation of the root helper (such as db.add) or an event that only the web side sees
// (such as login.ok); Kind is its group (see Category).
type Entry struct {
	ID     int64  `json:"id"`
	At     int64  `json:"at"` // Unix seconds
	IP     string `json:"ip"`
	Site   string `json:"site"`
	Op     string `json:"op"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Danger bool   `json:"danger,omitempty"` // a change the owner wants to hear about at once (see ctl.Dangerous)
}

// Filter chooses entries. Empty fields choose everything; From and To are days (2026-11-20) in the time zone of the machine, both
// included. IP matches the start of the address, so 2001:db8: finds a whole network.
type Filter struct {
	Site, IP, Kind, From, To string
	Limit, Offset            int
}

// Page is a part of the log, newest first, and how many entries the filter matches in all.
type Page struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
}

// Kinds are the groups of the log, in the order the page offers them.
var Kinds = []string{"auth", "site", "files", "terminal", "database", "ssl", "cron", "backup", "mail", "security", "system"}

// Events are what only the web side sees: signing in and out, the account, the terminal and the file manager. The root helper
// takes a note of one of these from the web side and refuses any other name.
var Events = map[string]bool{
	"login.ok": true, "login.fail": true, "login.2fa_fail": true, "logout": true,
	"account.setup": true, "account.password": true, "account.password_fail": true, "account.2fa_on": true, "account.2fa_off": true, "account.sessions_end": true,
	"terminal.open": true, "terminal.close": true,
	"files.upload": true, "files.mkdir": true, "files.newfile": true, "files.rename": true, "files.delete": true, "files.zip": true,
	"files.unzip": true, "files.chmod": true, "files.move": true, "files.copy": true, "files.save": true,
}

// Category is the group of an operation or an event.
func Category(op string) string {
	switch {
	case strings.HasPrefix(op, "login.") || op == "logout" || strings.HasPrefix(op, "account."):
		return "auth"
	case strings.HasPrefix(op, "files."):
		return "files"
	case strings.HasPrefix(op, "terminal."):
		return "terminal"
	case strings.HasPrefix(op, "db."):
		return "database"
	case strings.HasPrefix(op, "site.ssl"), op == "site.csr", strings.HasPrefix(op, "panel.ssl"):
		return "ssl"
	case strings.HasPrefix(op, "site.cron"):
		return "cron"
	case strings.HasPrefix(op, "backup."), strings.HasPrefix(op, "cloud."):
		return "backup"
	case strings.HasPrefix(op, "mail."):
		return "mail"
	case strings.HasPrefix(op, "firewall."), strings.HasPrefix(op, "security."), strings.HasPrefix(op, "port."):
		return "security"
	case strings.HasPrefix(op, "services."), strings.HasPrefix(op, "panel."), strings.HasPrefix(op, "notify."):
		return "system"
	}
	return "site"
}

// ValidKind accepts "" or one of Kinds.
func ValidKind(k string) error {
	if k == "" {
		return nil
	}
	for _, x := range Kinds {
		if x == k {
			return nil
		}
	}
	return errors.New("unknown kind of activity")
}

// ValidDay accepts "" or a day written 2026-11-20.
func ValidDay(d string) error {
	if d == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", d); err != nil || len(d) != 10 {
		return errors.New("a day is written YYYY-MM-DD")
	}
	return nil
}

// ValidIPFilter accepts "" or the start of an IPv4 or IPv6 address.
func ValidIPFilter(s string) error {
	if len(s) > 45 {
		return errors.New("the address is too long")
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' || c == '.' || c == ':') {
			return errors.New("an address has only digits, dots and colons")
		}
	}
	return nil
}

// ValidActor accepts "" or an IP address, which is how the web side names the person.
func ValidActor(s string) error {
	if s != "" && net.ParseIP(s) == nil {
		return errors.New("the address of the person is not valid")
	}
	return nil
}

// MaxDetail is the longest detail kept.
const MaxDetail = 500

// CleanDetail makes a detail safe to keep and to show: control characters (a line break could fake a second entry in an export)
// become spaces, and the text is cut at MaxDetail bytes, on a character boundary.
func CleanDetail(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if len(s) > MaxDetail {
		cut := MaxDetail
		for cut > 0 && !utfStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return strings.TrimSpace(s)
}

func utfStart(b byte) bool { return b&0xC0 != 0x80 }

// CSVSafe keeps a spreadsheet from reading a cell of an export as a formula.
func CSVSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// CSVHeader and CSVRow are the columns of an export of the log.
var CSVHeader = []string{"time", "ip", "site", "kind", "op", "detail", "ok", "error"}

func CSVRow(e Entry, loc *time.Location) []string {
	ok := "yes"
	if !e.OK {
		ok = "no"
	}
	return []string{time.Unix(e.At, 0).In(loc).Format(time.RFC3339), e.IP, CSVSafe(e.Site), e.Kind, e.Op, CSVSafe(e.Detail), ok, CSVSafe(e.Error)}
}
