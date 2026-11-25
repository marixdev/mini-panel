package web

import (
	"net/url"
	"strings"
	"testing"

	"mpn/internal/ctl"
)

func TestTheRemoteAccessPageShowsTheAddressesAndWarnsWithoutAFirewall(t *testing.T) {
	h, _, c, _ := setup(t, byOp(map[string]string{
		"db.remoteinfo": `{"name":"blog","hosts":["203.0.113.5","%"],"listening":true,"firewall_on":false,"port":3306}`,
	}))
	body := do(h, "GET", "/databases/remote?name=blog&site=blog", nil, cookie(c)).Body.String()
	for _, want := range []string{`action="/databases/remote"`, "203.0.113.5\n%</textarea>", "3306", `name="close"`, "Tường lửa đang tắt", "bất kỳ ai trên internet"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if do(h, "GET", "/databases/remote?name=x'y&site=blog", nil, cookie(c)).Code != 400 {
		t.Error("a name that is not a database name is refused")
	}
}

func TestSavingTheAddressesSendsThemNormalized(t *testing.T) {
	for _, tc := range []struct {
		form url.Values
		want string
	}{
		{url.Values{"hosts": {" 203.0.113.9/24\n203.0.113.9/24\r\n198.51.100.7 "}}, "203.0.113.0/24,198.51.100.7"},
		{url.Values{"hosts": {"203.0.113.5"}, "close": {"1"}}, ""},
		{url.Values{"hosts": {""}}, ""},
	} {
		h, f, c, tok := setup(t, byOp(map[string]string{"db.remote": "ok", "db.remoteinfo": `{"hosts":[],"port":3306}`}))
		tc.form.Set("csrf", tok)
		tc.form.Set("name", "blog")
		tc.form.Set("site", "blog")
		followed(h, do(h, "POST", "/databases/remote", tc.form, cookie(c)), c)
		req, ok := lastOp(f, "db.remote")
		if !ok || req.Name != "blog" || strings.Join(req.Hosts, ",") != tc.want {
			t.Errorf("%v: sent %+v", tc.form, req)
		}
	}
}

func TestABadAddressIsSaidOnThePageAndNothingIsSent(t *testing.T) {
	h, f, c, tok := setup(t, byOp(map[string]string{"db.remoteinfo": `{"hosts":[],"port":3306,"firewall_on":true}`}))
	w := do(h, "POST", "/databases/remote", url.Values{"csrf": {tok}, "name": {"blog"}, "site": {"blog"}, "hosts": {"db.example.com"}}, cookie(c))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "Không nhận tên miền") || !strings.Contains(w.Body.String(), "db.example.com</textarea>") {
		t.Errorf("%d\n%s", w.Code, w.Body.String())
	}
	if _, ok := lastOp(f, "db.remote"); ok {
		t.Error("nothing is sent for a bad address")
	}
}

func TestTheRequestTurnsIntoTheCommand(t *testing.T) {
	argv, err := ctl.Request{Op: "db.remote", Name: "blog", Hosts: []string{"203.0.113.0/24", "%"}}.Argv()
	if err != nil || strings.Join(argv, " ") != "db remote -name blog -hosts 203.0.113.0/24,% -apply" {
		t.Errorf("%v %v", argv, err)
	}
	argv, _ = ctl.Request{Op: "db.remote", Name: "blog"}.Argv()
	if strings.Join(argv, " ") != "db remote -name blog -hosts none -apply" {
		t.Errorf("no hosts closes remote access: %v", argv)
	}
	if _, err := (ctl.Request{Op: "db.remote", Name: "blog", Hosts: []string{"1.2.3.4 -apply"}}).Argv(); err == nil {
		t.Error("a host that is not an address never reaches the command line")
	}
}
