package web

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"mpn/internal/update"
)

const testChangelog = `# Changelog

## [1.3.0] - 2026-11-25

### Added

- **Remote access** to databases, see [docs](https://mpanel.biz/db/) and ` + "`mpn db remote`" + `.
- <script>alert(1)</script> and [bad](javascript:alert(1)).

## [1.2.4] - 2026-11-23

- A fix.

## [1.2.3] - 2026-11-22

- Already installed.
`

func fakeChecker(t *testing.T, running string, latest string, calls *int) *UpdateChecker {
	return &UpdateChecker{
		File:    filepath.Join(t.TempDir(), "update-check.json"),
		Current: func() string { return running },
		Latest: func(context.Context) (update.Version, error) {
			if latest == "" {
				return update.Version{}, errors.New("github is down")
			}
			return update.ParseVersion(latest)
		},
		Changelog: func(_ context.Context, v update.Version, lang string) (string, error) {
			if calls != nil {
				*calls++
			}
			if lang == "zh" {
				return "", errors.New("404")
			}
			return testChangelog, nil
		},
	}
}

func TestTheCheckerKeepsTheChangelogOfNewerReleasesOnly(t *testing.T) {
	calls := 0
	u := fakeChecker(t, "1.2.3", "1.3.0", &calls)
	info := u.Check(context.Background())
	en := info.Changelogs["en"]
	if info.Latest != "1.3.0" || len(en) != 2 || en[0].Version != "1.3.0" || en[1].Version != "1.2.4" {
		t.Fatalf("%+v", info)
	}
	if v, s := u.Available("en"); v != "1.3.0" || len(s) != 2 {
		t.Errorf("available: %q %d", v, len(s))
	}
	first := calls
	u.Check(context.Background())
	// zh is missing (404) and is asked again; en and vi are not read again for the same release.
	if calls-first != 1 {
		t.Errorf("the changelogs of the same release are read once, read %d more times", calls-first)
	}
	// A restart reads the file back.
	again := fakeChecker(t, "1.2.3", "1.3.0", nil)
	again.File = u.File
	again.Load()
	if v, _ := again.Available("vi"); v != "1.3.0" {
		t.Error("the answer is kept across a restart")
	}
	// Once the program runs the release, the notice goes away without asking GitHub.
	again.Current = func() string { return "1.3.0" }
	if v, _ := again.Available("en"); v != "" {
		t.Error("after the update nothing is offered")
	}
}

func TestAFailedCheckKeepsWhatWasKnownAndAMissingTranslationFallsBackToEnglish(t *testing.T) {
	u := fakeChecker(t, "1.2.3", "1.3.0", nil)
	info := u.Check(context.Background())
	if _, ok := info.Changelogs["zh"]; ok || len(info.Changelogs["vi"]) != 2 || info.Error != "" {
		t.Fatalf("%+v", info)
	}
	if _, s := u.Available("zh"); len(s) != 2 || s[0].Version != "1.3.0" {
		t.Errorf("zh falls back to English: %+v", s)
	}
	u.Latest = func(context.Context) (update.Version, error) { return update.Version{}, errors.New("github is down") }
	info = u.Check(context.Background())
	if info.Error == "" || info.Latest != "1.3.0" || len(info.Changelogs["en"]) != 2 {
		t.Errorf("%+v", info)
	}
}

func TestADevBuildIsNeverOfferedAnything(t *testing.T) {
	u := fakeChecker(t, "dev", "1.3.0", nil)
	u.Check(context.Background())
	if v, _ := u.Available("en"); v != "" {
		t.Error("a build without a version has nothing to compare")
	}
}

func TestTheChangelogIsDrawnWithoutMarkupFromIt(t *testing.T) {
	got := string(changelogHTML(update.Sections(testChangelog, update.Version{Major: 1, Minor: 3})[0].Body))
	for _, want := range []string{"<h4>Added</h4>", "<strong>Remote access</strong>", `<a class="link" href="https://mpanel.biz/db/"`, "<code>mpn db remote</code>", "&lt;script&gt;"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "<script>") || strings.Contains(got, "javascript:") {
		t.Errorf("markup got through: %s", got)
	}
}

func TestEveryPageShowsTheNoticeAndTheUpdatePageTheChangelog(t *testing.T) {
	u := fakeChecker(t, "1.2.3", "1.3.0", nil)
	u.Check(context.Background())
	h, f, c, csrf := setupWith(t, byOp(map[string]string{"site.list": "[]", "panel.update": "started"}), func(s *Server) { s.Updates = u })
	home := do(h, "GET", "/", nil, cookie(c)).Body.String()
	if !strings.Contains(home, `data-up-float data-version="1.3.0"`) || !strings.Contains(home, `href="/update" class="up-float-link"`) {
		t.Error("the home page has no notice")
	}
	page := do(h, "GET", "/update", nil, cookie(c)).Body.String()
	for _, want := range []string{"data-changelog", "<strong>Remote access</strong>", "1.2.4", `name="version" value="1.3.0"`} {
		if !strings.Contains(page, want) {
			t.Errorf("update page: missing %q", want)
		}
	}
	if strings.Contains(page, "data-up-float") {
		t.Error("the update page itself has no floating notice")
	}
	if strings.Contains(page, "Already installed") {
		t.Error("the changelog of the running release is not shown")
	}
	do(h, "POST", "/update", url.Values{"csrf": {csrf}, "version": {"1.3.0"}}, cookie(c))
	if r, ok := lastOp(f, "panel.update"); !ok || r.Version != "1.3.0" {
		t.Errorf("the button installs that release: %+v", r)
	}
	// After the update the program runs 1.3.0: no notice anywhere.
	u.Current = func() string { return "1.3.0" }
	if strings.Contains(do(h, "GET", "/", nil, cookie(c)).Body.String(), "data-up-float") {
		t.Error("the notice is gone once the release runs")
	}
	if p := do(h, "GET", "/update", nil, cookie(c)).Body.String(); strings.Contains(p, "data-changelog") || !strings.Contains(p, "data-up-state") {
		t.Error("the update page says this is the latest")
	}
}

func TestCheckNowAsksGitHubWithoutTheHelper(t *testing.T) {
	calls := 0
	u := fakeChecker(t, "1.2.3", "1.2.3", nil)
	u.Latest = func(context.Context) (update.Version, error) { calls++; return update.ParseVersion("1.2.4") }
	h, f, c, csrf := setupWith(t, byOp(map[string]string{}), func(s *Server) { s.Updates = u })
	w := do(h, "POST", "/update/check", url.Values{"csrf": {csrf}}, cookie(c))
	if w.Code != 303 || w.Header().Get("Location") != "/update" || calls != 1 {
		t.Errorf("%d %s calls=%d", w.Code, w.Header().Get("Location"), calls)
	}
	if _, ok := lastOp(f, "panel.updatecheck"); ok {
		t.Error("the helper is not asked")
	}
	if do(h, "POST", "/update/check", url.Values{}, cookie(c)).Code == 303 && calls != 1 {
		t.Error("without the form token nothing is asked")
	}
}
