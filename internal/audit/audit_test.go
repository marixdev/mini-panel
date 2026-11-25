package audit

import (
	"strings"
	"testing"
	"time"
)

func TestCategoryGroupsEveryKindOfAction(t *testing.T) {
	for op, want := range map[string]string{
		"login.ok": "auth", "logout": "auth", "account.password": "auth", "files.delete": "files", "terminal.open": "terminal",
		"db.add": "database", "site.ssl": "ssl", "site.csr": "ssl", "panel.sslcustom": "ssl", "site.cronadd": "cron",
		"backup.restore": "backup", "backup.keep": "backup", "cloud.sync": "backup", "mail.boxadd": "mail", "firewall.del": "security",
		"security.waf": "security", "services.control": "system", "panel.update": "system", "site.add": "site", "wp.run": "site",
	} {
		if got := Category(op); got != want {
			t.Errorf("%s: %s, want %s", op, got, want)
		}
		if ValidKind(Category(op)) != nil {
			t.Errorf("%s: the kind %s is not offered", op, Category(op))
		}
	}
	for e := range Events {
		if ValidKind(Category(e)) != nil {
			t.Errorf("event %s has no kind", e)
		}
	}
}

// A line break in a name could make a second, false line in an export; a long text is cut on a character, not in the middle of one.
func TestCleanDetailKeepsOneLineAndWholeCharacters(t *testing.T) {
	if got := CleanDetail("a\nb\r\tc"); got != "a b  c" {
		t.Errorf("%q", got)
	}
	long := CleanDetail(strings.Repeat("ữ", 400))
	if len(long) > MaxDetail+len("…") || !strings.HasSuffix(long, "…") || strings.ContainsRune(long, '�') {
		t.Errorf("cut badly: %d bytes", len(long))
	}
}

func TestExportCannotStartAFormula(t *testing.T) {
	for _, s := range []string{"=1+1", "+x", "-x", "@x"} {
		if CSVSafe(s)[0] != '\'' {
			t.Errorf("%q is not neutralised", s)
		}
	}
	row := CSVRow(Entry{At: 1, IP: "1.1.1.1", Site: "blog", Op: "db.add", Kind: "database", Detail: "=HYPERLINK()", OK: true}, time.UTC)
	if row[5] != "'=HYPERLINK()" || row[6] != "yes" || len(row) != len(CSVHeader) {
		t.Errorf("%q", row)
	}
}

func TestFiltersRefuseWhatIsNotAnAddressOrADay(t *testing.T) {
	for _, s := range []string{"1.1.1.1", "2001:db8:", "10.0.", ""} {
		if ValidIPFilter(s) != nil {
			t.Errorf("%q refused", s)
		}
	}
	for _, s := range []string{"1.1.1.1%", "a_b", "' OR 1=1", strings.Repeat("1", 46)} {
		if ValidIPFilter(s) == nil {
			t.Errorf("%q accepted", s)
		}
	}
	if ValidDay("2026-11-20") != nil || ValidDay("2026-1-2") == nil || ValidDay("20261120") == nil || ValidKind("nope") == nil {
		t.Error("day or kind check is wrong")
	}
	if ValidActor("1.2.3.4") != nil || ValidActor("::1") != nil || ValidActor("1.2.3.4 x") == nil {
		t.Error("actor check is wrong")
	}
}
