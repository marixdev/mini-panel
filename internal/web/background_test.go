package web

import (
	"net/url"
	"strings"
	"testing"

	"mpn/internal/ctl"
)

const bgServicesJSON = `[{"id":2,"name":"queue","args":"artisan queue:work --sleep=3","enabled":true,"memory_mb":256,"state":"active"},{"id":5,"name":"bot","args":"bot.js","enabled":false,"memory_mb":0,"state":"inactive"}]`

// bgAnswers answers the helper for a PHP site. The services list is the same for every read.
func bgAnswers(list string) func(ctl.Request) (ctl.Response, error) {
	return byOp(map[string]string{"site.list": siteListJSON("", "", false), "site.bglist": list, "site.bgadd": "added", "site.bgset": "set", "site.bgdel": "gone", "site.bgrestart": "restarted", "site.info": phpInfo()})
}

func TestTheBackgroundTabListsServicesAndOffersTheActions(t *testing.T) {
	h, _, c, _ := setupWith(t, bgAnswers(bgServicesJSON), nil)
	body := do(h, "GET", "/sites/shop/background", nil, cookie(c)).Body.String()
	for _, want := range []string{
		"queue", "artisan queue:work --sleep=3", "bot.js", "256 MB",
		`action="/sites/shop/background"`, `name="name"`, `name="args"`, `name="memory_mb"`, `name="enabled"`,
		`action="/sites/shop/background/2/toggle"`, `action="/sites/shop/background/5/toggle"`,
		`action="/sites/shop/background/2/restart"`, `action="/sites/shop/background/5/delete"`,
		`name="on" value="0"`, `name="on" value="1"`, "Đang chạy", "Đã dừng",
		`href="/sites/shop/background"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	h, _, c, _ = setupWith(t, bgAnswers("[]"), nil)
	if body = do(h, "GET", "/sites/shop/background", nil, cookie(c)).Body.String(); !strings.Contains(body, "Chưa có dịch vụ nền nào") {
		t.Error("an empty list says so")
	}
}

func TestAddingAServiceSendsItsSettings(t *testing.T) {
	h, f, c, tok := setupWith(t, bgAnswers("[]"), nil)
	form := url.Values{"csrf": {tok}, "name": {"  queue "}, "args": {"artisan queue:work --sleep=3"}, "memory_mb": {"256"}, "enabled": {"1"}}
	w := followed(h, do(h, "POST", "/sites/shop/background", form, cookie(c)), c)
	req, ok := lastOp(f, "site.bgadd")
	if !ok || req.Site != "shop" || req.BgName != "queue" || req.BgArgs != "artisan queue:work --sleep=3" || req.BgMemoryMB != 256 || !req.BgEnabled {
		t.Fatalf("%+v\n%s", req, w.Body.String())
	}
	if backOf(t, w.Body.String()) != "/sites/shop/background" {
		t.Errorf("back: %s", w.Body.String())
	}
}

func TestAServiceThatIsNotValidNeverReachesTheHelper(t *testing.T) {
	for name, form := range map[string]url.Values{
		"an injected argument": {"name": {"q"}, "args": {"w.js;rm -rf /"}, "memory_mb": {"0"}, "enabled": {"1"}},
		"a substitution":       {"name": {"q"}, "args": {"$(id)"}, "memory_mb": {"0"}, "enabled": {"1"}},
		"an absolute path":     {"name": {"q"}, "args": {"/usr/bin/x"}, "memory_mb": {"0"}, "enabled": {"1"}},
		"a dot dot path":       {"name": {"q"}, "args": {"../x.js"}, "memory_mb": {"0"}, "enabled": {"1"}},
		"a bad name":           {"name": {"a;b"}, "args": {"w.js"}, "memory_mb": {"0"}, "enabled": {"1"}},
		"memory too small":     {"name": {"q"}, "args": {"w.js"}, "memory_mb": {"1"}, "enabled": {"1"}},
		"memory not a number":  {"name": {"q"}, "args": {"w.js"}, "memory_mb": {"lots"}, "enabled": {"1"}},
	} {
		h, f, c, tok := setupWith(t, bgAnswers("[]"), nil)
		form.Set("csrf", tok)
		w := do(h, "POST", "/sites/shop/background", form, cookie(c))
		if _, sent := lastOp(f, "site.bgadd"); sent || w.Code != 400 {
			t.Errorf("%s: sent=%v code=%d", name, sent, w.Code)
		}
	}
}

func TestToggleKeepsTheSettingsOfTheService(t *testing.T) {
	h, f, c, tok := setupWith(t, bgAnswers(bgServicesJSON), nil)
	followed(h, do(h, "POST", "/sites/shop/background/2/toggle", url.Values{"csrf": {tok}, "on": {"0"}}, cookie(c)), c)
	req, ok := lastOp(f, "site.bgset")
	if !ok || req.BgID != 2 || req.BgEnabled || req.BgName != "queue" || req.BgArgs != "artisan queue:work --sleep=3" || req.BgMemoryMB != 256 {
		t.Errorf("pause must keep the other settings: %+v", req)
	}
	followed(h, do(h, "POST", "/sites/shop/background/5/toggle", url.Values{"csrf": {tok}, "on": {"1"}}, cookie(c)), c)
	if req, _ := lastOp(f, "site.bgset"); req.BgID != 5 || !req.BgEnabled || req.BgName != "bot" {
		t.Errorf("start: %+v", req)
	}
}

func TestRestartAndDeleteOfAService(t *testing.T) {
	h, f, c, tok := setupWith(t, bgAnswers(bgServicesJSON), nil)
	followed(h, do(h, "POST", "/sites/shop/background/2/restart", url.Values{"csrf": {tok}}, cookie(c)), c)
	if req, ok := lastOp(f, "site.bgrestart"); !ok || req.BgID != 2 {
		t.Errorf("restart: %+v", req)
	}
	followed(h, do(h, "POST", "/sites/shop/background/5/delete", url.Values{"csrf": {tok}}, cookie(c)), c)
	if req, ok := lastOp(f, "site.bgdel"); !ok || req.BgID != 5 {
		t.Errorf("delete: %+v", req)
	}
}

func TestBackgroundPostsNeedAValidNumberAndTheToken(t *testing.T) {
	h, f, c, tok := setupWith(t, bgAnswers(bgServicesJSON), nil)
	before := len(f.reqs)
	for _, p := range []string{"/sites/shop/background/x/delete", "/sites/shop/background/0/restart", "/sites/shop/background/-1/toggle"} {
		if w := do(h, "POST", p, url.Values{"csrf": {tok}}, cookie(c)); w.Code != 400 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := do(h, "POST", "/sites/shop/background/2/delete", url.Values{"csrf": {"wrong"}}, cookie(c)); w.Code != 403 {
		t.Errorf("wrong token: %d", w.Code)
	}
	if w := do(h, "POST", "/sites/nope/background", url.Values{"csrf": {tok}, "name": {"q"}, "args": {"w.js"}}, cookie(c)); w.Code != 404 {
		t.Errorf("unknown site: %d", w.Code)
	}
	for _, r := range f.reqs[before:] {
		if strings.HasPrefix(r.Op, "site.bg") && r.Op != "site.bglist" && r.Op != "site.info" {
			t.Errorf("%s must not reach the helper", r.Op)
		}
	}
}

func TestStaticSitesHaveNoBackgroundTab(t *testing.T) {
	answers := byOp(map[string]string{"site.list": siteListJSON("", "", false), "site.info": phpInfo()})
	h, _, c, _ := setupWith(t, answers, nil)
	// the "other" site of the list is static; its tab must not exist and its page must not offer services
	body := do(h, "GET", "/sites/other", nil, cookie(c)).Body.String()
	if strings.Contains(body, `href="/sites/other/background"`) {
		t.Error("a static site must not show the Background tab")
	}
}

// The tab shows the state that the bglist answer carries for each service, in the words of the locale.
func TestTheBackgroundTabShowsEachServiceState(t *testing.T) {
	const list = `[{"id":1,"name":"a","args":"a.php","enabled":true,"memory_mb":0,"state":"active"},` +
		`{"id":2,"name":"b","args":"b.php","enabled":true,"memory_mb":0,"state":"failed"},` +
		`{"id":3,"name":"c","args":"c.php","enabled":false,"memory_mb":0,"state":"inactive"}]`
	h, _, c, _ := setupWith(t, bgAnswers(list), nil)
	body := do(h, "GET", "/sites/shop/background", nil, cookie(c)).Body.String()
	for _, want := range []string{"Đang chạy", "Lỗi", "Đã dừng"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing state label %q", want)
		}
	}
	if strings.Contains(body, "Chưa rõ") {
		t.Error("every service has a known state, so no service may show unknown")
	}
}
