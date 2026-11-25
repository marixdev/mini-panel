package web

import (
	"net/url"
	"reflect"
	"strings"
	"testing"

	"mpn/internal/ctl"
)

const backupListJSON = `[
{"site":"blog","backup":"20261104-031500","state":"ok","files":true,"databases":["blog","blog_x"],"size_mb":1.5},
{"site":"blog","backup":"20261104-020000","state":"incomplete","files":false,"databases":[],"size_mb":0},
{"site":"a<b>","backup":"20261103-010000","state":"ok","files":true,"databases":[],"size_mb":0.2}]`

func backupHelper() map[string]string {
	return map[string]string{"backup.list": backupListJSON, "backup.create": "saved", "backup.restore": "restored", "backup.prune": "pruned",
		"site.list": `[{"name":"blog","kind":"php","user":"blog","hostnames":["b.example.com"],"detail":"","tls":false}]`}
}

func TestBackupsTabListsBackupsAndOnlyOffersRestoreForFinishedOnes(t *testing.T) {
	h, _, c, _ := setup(t, byOp(backupHelper()))
	body := do(h, "GET", "/sites/blog/backups", nil, cookie(c)).Body.String()
	for _, want := range []string{"/backups/restore?site=blog&amp;backup=20261104-031500", "1.5 MB", "db:blog_x"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "backup=20261104-020000") || strings.Contains(body, "<b>") {
		t.Error("an unfinished backup must have no restore link, and names must be escaped")
	}
}

func TestCreatingABackupRunsTheHelperAndShowsItsAnswer(t *testing.T) {
	h, f, c, tok := setup(t, byOp(backupHelper()))
	w := followed(h, do(h, "POST", "/backups/create", url.Values{"csrf": {tok}, "site": {"blog"}}, cookie(c)), c)
	if len(f.reqs) != 1 || f.reqs[0].Op != "backup.create" || f.reqs[0].Site != "blog" || !strings.Contains(w.Body.String(), "saved") {
		t.Errorf("%+v\n%s", f.reqs, w.Body.String())
	}
}

func TestRestoreConfirmationOffersWhatTheBackupHolds(t *testing.T) {
	h, _, c, _ := setup(t, byOp(backupHelper()))
	page := do(h, "GET", "/backups/restore?site=blog&backup=20261104-031500", nil, cookie(c)).Body.String()
	for _, want := range []string{`name="files"`, `name="database" value="blog"`, `name="database" value="blog_x"`, "<b>blog</b>", `name="backup" value="20261104-031500"`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q in:\n%s", want, page)
		}
	}
	for path, want := range map[string]int{
		"/backups/restore?site=blog&backup=20261104-020000": 404, // unfinished
		"/backups/restore?site=blog&backup=20250101-000000": 404, // unknown
		"/backups/restore?site=blog&backup=..%2Fx":          400,
		"/backups/restore?site=-x&backup=20261104-031500":   400,
	} {
		if w := do(h, "GET", path, nil, cookie(c)); w.Code != want {
			t.Errorf("%s: %d, want %d", path, w.Code, want)
		}
	}
}

func TestRestoreNeedsTheNameTypedAndSomethingChosen(t *testing.T) {
	h, f, c, tok := setup(t, byOp(backupHelper()))
	form := func(confirm string, more url.Values) url.Values {
		v := url.Values{"csrf": {tok}, "site": {"blog"}, "backup": {"20261104-031500"}, "confirm": {confirm}}
		for k, x := range more {
			v[k] = x
		}
		return v
	}
	if w := do(h, "POST", "/backups/restore", form("blo", url.Values{"files": {"on"}}), cookie(c)); w.Code != 400 {
		t.Errorf("wrong confirmation: %d", w.Code)
	}
	if w := do(h, "POST", "/backups/restore", form("blog", nil), cookie(c)); w.Code != 400 {
		t.Errorf("nothing chosen: %d", w.Code)
	}
	if w := do(h, "POST", "/backups/restore", form("blog", url.Values{"backup": {"../x"}, "files": {"on"}}), cookie(c)); w.Code != 400 {
		t.Errorf("bad backup name: %d", w.Code)
	}
	if len(f.reqs) != 0 {
		t.Fatalf("refused forms reached the helper: %+v", f.reqs)
	}
	do(h, "POST", "/backups/restore", form("blog", url.Values{"files": {"on"}, "database": {"blog", "blog_x"}}), cookie(c))
	want := ctl.Request{Op: "backup.restore", Site: "blog", Backup: "20261104-031500", Files: true, Databases: []string{"blog", "blog_x"}, Actor: "192.0.2.1"}
	if len(f.reqs) != 1 || !reflect.DeepEqual(f.reqs[0], want) {
		t.Errorf("got %+v, want %+v", f.reqs, want)
	}
}

func TestPruningNeedsTheNameTypedAndASaneNumber(t *testing.T) {
	h, f, c, tok := setup(t, byOp(backupHelper()))
	post := func(keep, confirm string) int {
		return do(h, "POST", "/backups/prune", url.Values{"csrf": {tok}, "site": {"blog"}, "keep": {keep}, "confirm": {confirm}}, cookie(c)).Code
	}
	if post("7", "bl") != 400 || post("0", "blog") != 400 || post("abc", "blog") != 400 || post("9999", "blog") != 400 || len(f.reqs) != 0 {
		t.Fatalf("refused forms must not reach the helper: %+v", f.reqs)
	}
	if post("7", "blog") != 303 || len(f.reqs) != 1 || f.reqs[0].Op != "backup.prune" || f.reqs[0].Keep != 7 || f.reqs[0].Site != "blog" {
		t.Errorf("%+v", f.reqs)
	}
}

func TestTheNumberOfBackupsKeptIsShownAndSaved(t *testing.T) {
	ops := backupHelper()
	ops["backup.keepinfo"] = `{"keep":3}`
	ops["backup.keep"] = "kept"
	h, f, c, tok := setup(t, byOp(ops))
	body := do(h, "GET", "/sites/blog/backups", nil, cookie(c)).Body.String()
	for _, want := range []string{`action="/backups/keep"`, `name="keep" type="number" min="1" max="365" required value="3"`, "3"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	f.reqs = nil
	post := func(keep string) int {
		return do(h, "POST", "/backups/keep", url.Values{"csrf": {tok}, "site": {"blog"}, "keep": {keep}}, cookie(c)).Code
	}
	if post("0") != 400 || post("366") != 400 || post("x") != 400 || len(f.reqs) != 0 {
		t.Fatalf("refused numbers must not reach the helper: %+v", f.reqs)
	}
	if post("4") != 303 || len(f.reqs) != 1 || f.reqs[0].Op != "backup.keep" || f.reqs[0].Keep != 4 || f.reqs[0].Site != "blog" {
		t.Errorf("%+v", f.reqs)
	}
}

func TestBackupPagesNeedASessionAndAToken(t *testing.T) {
	h, f, c, _ := setup(t, byOp(backupHelper()))
	for _, p := range []string{"/sites/blog/backups", "/backups/restore?site=blog&backup=20261104-031500"} {
		if w := do(h, "GET", p, nil); w.Code != 303 {
			t.Errorf("%s without a session: %d", p, w.Code)
		}
	}
	for _, p := range []string{"/backups/create", "/backups/restore", "/backups/prune", "/backups/keep"} {
		if w := do(h, "POST", p, url.Values{"site": {"blog"}, "confirm": {"blog"}, "keep": {"7"}, "files": {"on"}}, cookie(c)); w.Code != 403 {
			t.Errorf("%s without a token: %d", p, w.Code)
		}
	}
	if len(f.reqs) != 0 {
		t.Errorf("refused requests reached the helper: %+v", f.reqs)
	}
}
