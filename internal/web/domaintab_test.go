package web

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"mpn/internal/ctl"
)

func siteListJSON(mode, www string, allowHTTP bool) string {
	names := `["example.com"]`
	switch www {
	case "bare":
		names = `["example.com","www.example.com"]`
	case "www":
		names = `["www.example.com","example.com"]`
	}
	return fmt.Sprintf(`[{"name":"shop","kind":"php","user":"shop","hostnames":%s,"detail":"php 8.4","tls":%v,"root":"/home/shop/htdocs","www":%q,"allow_http":%v,"ssl_mode":%q},
{"name":"other","kind":"static","user":"other","hostnames":["taken.example.org"],"detail":"","tls":false,"root":"/home/other/htdocs"}]`, names, mode != "", www, allowHTTP, mode)
}

func sslInfoJSON(mode string, days int, covers, trusted bool, csr string) string {
	if mode == "" {
		return fmt.Sprintf(`{"mode":"","covers":false,"trusted":false,"pending_csr":%q}`, csr)
	}
	exp := time.Now().Add(time.Duration(days)*24*time.Hour + time.Hour).Unix()
	return fmt.Sprintf(`{"mode":%q,"issuer":"Test CA","names":["example.com","www.example.com"],"not_before":%d,"not_after":%d,"covers":%v,"trusted":%v,"pending_csr":%q}`,
		mode, time.Now().Add(-24*time.Hour).Unix(), exp, covers, trusted, csr)
}

func lastOp(f *fakeHelper, op string) (ctl.Request, bool) {
	for i := len(f.reqs) - 1; i >= 0; i-- {
		if f.reqs[i].Op == op {
			return f.reqs[i], true
		}
	}
	return ctl.Request{}, false
}

func sslAnswers(list, info string) func(ctl.Request) (ctl.Response, error) {
	return byOp(map[string]string{"site.list": list, "site.sslinfo": info, "site.domain": "done", "site.ssl": "done", "site.sslcustom": "done",
		"site.csr": "done", "site.ssloff": "done", "site.sslredirect": "done", "site.info": phpInfo()})
}

func TestTheOverviewHasTheDomainCardAndWarnsOnlyWhenSSLIsOn(t *testing.T) {
	h, _, c, _ := setupWith(t, sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, "")), nil)
	body := do(h, "GET", "/sites/shop", nil, cookie(c)).Body.String()
	for _, want := range []string{`action="/sites/shop/domain"`, `name="domain"`, `value="example.com"`, `name="www"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "SSL hiện tại bị gỡ") || strings.Contains(body, `name="redirect_https"`) {
		t.Error("a site without SSL has nothing to lose, and the redirect belongs to the SSL tab")
	}
	h, _, c, _ = setupWith(t, sslAnswers(siteListJSON("letsencrypt", "bare", false), sslInfoJSON("letsencrypt", 60, true, true, "")), nil)
	body = do(h, "GET", "/sites/shop", nil, cookie(c)).Body.String()
	if !strings.Contains(body, "SSL hiện tại bị gỡ") || !strings.Contains(body, `value="bare" class="mt-1 accent-primary" checked`) {
		t.Errorf("a site with SSL says what changing the domain costs, and shows the stored www choice")
	}
}

func TestNoDomainTabAndTheMenuHasSSL(t *testing.T) {
	h, _, c, _ := setupWith(t, sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, "")), nil)
	body := do(h, "GET", "/sites/shop", nil, cookie(c)).Body.String()
	if !strings.Contains(body, `href="/sites/shop/ssl"`) || strings.Contains(body, `href="/sites/shop/domain"`) {
		t.Error("the menu has an SSL tab and no domain tab")
	}
	if w := do(h, "GET", "/sites/shop/domain", nil, cookie(c)); w.Code == 200 {
		t.Error("there is no domain page any more")
	}
	if strings.Contains(body, "HTTPS") {
		t.Error("the interface says SSL, not HTTPS")
	}
}

func TestTheSSLTabOffersBothSourcesUntilThereIsACertificate(t *testing.T) {
	h, _, c, _ := setupWith(t, sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, "")), nil)
	body := do(h, "GET", "/sites/shop/ssl", nil, cookie(c)).Body.String()
	for _, want := range []string{"Chưa bật SSL", `action="/sites/shop/ssl/letsencrypt"`, `action="/sites/shop/ssl/custom"`, `name="cert"`, `name="key"`, `name="chain"`, `action="/sites/shop/ssl/csr"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, not := range []string{`action="/sites/shop/ssl/off"`, `action="/sites/shop/ssl/redirect"`} {
		if strings.Contains(body, not) {
			t.Errorf("a site without a certificate must not offer %q", not)
		}
	}
}

func TestALetsEncryptCertificateRenewsItselfAndAnOwnOneIsReplacedByHand(t *testing.T) {
	h, _, c, _ := setupWith(t, sslAnswers(siteListJSON("letsencrypt", "", false), sslInfoJSON("letsencrypt", 60, true, true, "")), nil)
	body := do(h, "GET", "/sites/shop/ssl", nil, cookie(c)).Body.String()
	for _, want := range []string{"SSL đang bật", "Let&#39;s Encrypt", "Test CA", "Tự gia hạn", `action="/sites/shop/ssl/redirect"`, `action="/sites/shop/ssl/off"`, "Thay chứng chỉ"} {
		if !strings.Contains(body, want) && !strings.Contains(strings.ReplaceAll(body, "&#39;", "'"), strings.ReplaceAll(want, "&#39;", "'")) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, `action="/sites/shop/ssl/letsencrypt"`) || strings.Contains(body, "Sắp hết hạn") {
		t.Error("a Let's Encrypt site has no button to get one, and nothing is about to end")
	}

	h, _, c, _ = setupWith(t, sslAnswers(siteListJSON("custom", "", true), sslInfoJSON("custom", 12, false, false, "")), nil)
	body = do(h, "GET", "/sites/shop/ssl", nil, cookie(c)).Body.String()
	for _, want := range []string{"Chứng chỉ riêng", "Sắp hết hạn", "không tự gia hạn", `action="/sites/shop/ssl/letsencrypt"`, "Chuyển sang", "không phủ đủ", "không tin"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "Tự gia hạn trước") {
		t.Error("an own certificate is not renewed by mpn")
	}
	if strings.Contains(body, `name="redirect" value="1" class="mt-1 size-4 accent-teal-600" checked`) {
		t.Error("this site allows plain HTTP, so the box must not be ticked")
	}
}

func TestAPendingRequestIsShownForCopying(t *testing.T) {
	csr := "-----BEGIN CERTIFICATE REQUEST-----\nABC\n-----END CERTIFICATE REQUEST-----\n"
	h, _, c, _ := setupWith(t, sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, csr)), nil)
	body := do(h, "GET", "/sites/shop/ssl", nil, cookie(c)).Body.String()
	if !strings.Contains(body, "BEGIN CERTIFICATE REQUEST") || !strings.Contains(body, "data-copy=") || !strings.Contains(body, "<details class=\"card mt-4\" open>") {
		t.Errorf("the request must be visible and copyable")
	}
}

func TestSavingTheDomainSendsOnlyTheDomainAndTheWWWChoice(t *testing.T) {
	h, f, c, tok := setupWith(t, sslAnswers(siteListJSON("letsencrypt", "", false), sslInfoJSON("letsencrypt", 60, true, true, "")), nil)
	form := url.Values{"csrf": {tok}, "domain": {" HTTPS://New-Shop.Example.com/path?x=1 "}, "www": {"www"}, "redirect_https": {"1"}, "issue": {"1"}}
	w := followed(h, do(h, "POST", "/sites/shop/domain", form, cookie(c)), c)
	req, ok := lastOp(f, "site.domain")
	if !ok || req.Site != "shop" || req.Domain != "new-shop.example.com" || req.WWW != "www" {
		t.Fatalf("%+v\n%s", req, w.Body.String())
	}
	if backOf(t, w.Body.String()) != "/sites/shop" {
		t.Errorf("back: %s", w.Body.String())
	}
}

func TestADomainThatIsWrongOrTakenNeverReachesTheHelper(t *testing.T) {
	for name, form := range map[string]url.Values{
		"empty":             {"domain": {""}},
		"not a host name":   {"domain": {"not a domain"}},
		"no dot":            {"domain": {"localhost"}},
		"a command":         {"domain": {"x.com;id"}},
		"an option":         {"domain": {"-x.com"}},
		"a bad www":         {"domain": {"example.com"}, "www": {"both"}},
		"another sites":     {"domain": {"taken.example.org"}},
		"another sites www": {"domain": {"www.taken.example.org"}, "www": {"bare"}},
	} {
		h, f, c, tok := setupWith(t, sslAnswers(siteListJSON("letsencrypt", "", false), sslInfoJSON("", 0, false, false, "")), nil)
		form.Set("csrf", tok)
		w := do(h, "POST", "/sites/shop/domain", form, cookie(c))
		if _, sent := lastOp(f, "site.domain"); sent || w.Code != 400 || !strings.Contains(w.Body.String(), "Chưa lưu tên miền") {
			t.Errorf("%s: sent=%v code=%d", name, sent, w.Code)
		}
	}
}

func TestEachSSLActionSendsItsOwnOperation(t *testing.T) {
	cases := []struct {
		path string
		form url.Values
		op   string
		chk  func(ctl.Request) bool
	}{
		{"/sites/shop/ssl/letsencrypt", url.Values{}, "site.ssl", func(r ctl.Request) bool { return r.Site == "shop" && r.CFToken == "" }},
		{"/sites/shop/ssl/cloudflare", url.Values{"token": {" abcdefghijklmnopqrstuvwxyz0123456789ABCD "}}, "site.ssl", func(r ctl.Request) bool {
			return r.Site == "shop" && r.CFToken == "abcdefghijklmnopqrstuvwxyz0123456789ABCD"
		}},
		{"/sites/shop/ssl/off", url.Values{}, "site.ssloff", func(r ctl.Request) bool { return r.Site == "shop" }},
		{"/sites/shop/ssl/redirect", url.Values{"redirect": {"1"}}, "site.sslredirect", func(r ctl.Request) bool { return r.Redirect }},
		{"/sites/shop/ssl/redirect", url.Values{}, "site.sslredirect", func(r ctl.Request) bool { return !r.Redirect }},
		{"/sites/shop/ssl/csr", url.Values{"algo": {"rsa"}}, "site.csr", func(r ctl.Request) bool { return r.Algo == "rsa" }},
		{"/sites/shop/ssl/custom", url.Values{"cert": {" -----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE----- "}, "key": {"-----BEGIN PRIVATE KEY-----\nk\n-----END PRIVATE KEY-----"}, "chain": {""}, "allow_untrusted": {"1"}},
			"site.sslcustom", func(r ctl.Request) bool {
				return strings.HasPrefix(r.Cert, "-----BEGIN CERTIFICATE") && strings.HasSuffix(r.Cert, "CERTIFICATE-----") && r.AllowUntrusted && strings.Contains(r.Key, "PRIVATE KEY")
			}},
	}
	for _, tc := range cases {
		h, f, c, tok := setupWith(t, sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, "")), nil)
		tc.form.Set("csrf", tok)
		w := followed(h, do(h, "POST", tc.path, tc.form, cookie(c)), c)
		req, ok := lastOp(f, tc.op)
		if !ok || !tc.chk(req) {
			t.Errorf("%s: %+v", tc.path, req)
		}
		if backOf(t, w.Body.String()) != "/sites/shop/ssl" {
			t.Errorf("%s goes back to %q", tc.path, backOf(t, w.Body.String()))
		}
	}
}

func TestACertificateFormThatCannotWorkIsRefusedBeforeTheHelper(t *testing.T) {
	pemCert := "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----"
	for name, form := range map[string]url.Values{
		"no certificate":    {"cert": {""}},
		"not a certificate": {"cert": {"hello"}},
		"a key that is not": {"cert": {pemCert}, "key": {"hello"}},
		"a passphrase key":  {"cert": {pemCert}, "key": {"-----BEGIN ENCRYPTED PRIVATE KEY-----\nx\n-----END ENCRYPTED PRIVATE KEY-----"}},
	} {
		h, f, c, tok := setupWith(t, sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, "")), nil)
		form.Set("csrf", tok)
		w := do(h, "POST", "/sites/shop/ssl/custom", form, cookie(c))
		if _, sent := lastOp(f, "site.sslcustom"); sent || w.Code != 400 || !strings.Contains(w.Body.String(), "Chưa thực hiện") {
			t.Errorf("%s: sent=%v code=%d", name, sent, w.Code)
		}
	}
	h, f, c, tok := setupWith(t, sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, "")), nil)
	if w := do(h, "POST", "/sites/shop/ssl/csr", url.Values{"csrf": {tok}, "algo": {"dsa"}}, cookie(c)); w.Code != 400 {
		t.Errorf("a key type that is not offered: %d", w.Code)
	}
	if _, sent := lastOp(f, "site.csr"); sent {
		t.Error("it reached the helper")
	}
}

func TestSSLFailuresAreExplainedInVietnamese(t *testing.T) {
	for output, want := range map[string]string{
		"dns-check-failed: example.com: it points to 198.51.100.9, which is not this server": "chưa trỏ về máy chủ",
		"ssl-untrusted: the certificate does not chain":                                      "CA bundle",
		"acme: boom": "Let's Encrypt từ chối",
	} {
		h, _, c, tok := setupWith(t, func(r ctl.Request) (ctl.Response, error) {
			if r.Op == "site.ssl" || r.Op == "site.sslcustom" {
				return ctl.Response{Output: output, Error: "exit status 2"}, nil
			}
			return sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, ""))(r)
		}, nil)
		path, form := "/sites/shop/ssl/letsencrypt", url.Values{"csrf": {tok}}
		if strings.HasPrefix(output, "ssl-untrusted") {
			path, form = "/sites/shop/ssl/custom", url.Values{"csrf": {tok}, "cert": {"-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----"}}
		}
		body := strings.ReplaceAll(followed(h, do(h, "POST", path, form, cookie(c)), c).Body.String(), "&#39;", "'")
		if !strings.Contains(body, want) {
			t.Errorf("%q: missing %q in\n%s", output, want, body)
		}
	}
}

func TestSSLNeedsALoginTheTokenAndAKnownSite(t *testing.T) {
	h, f, c, tok := setupWith(t, sslAnswers(siteListJSON("", "", false), sslInfoJSON("", 0, false, false, "")), nil)
	for name, tc := range map[string]struct {
		path   string
		form   url.Values
		cookie bool
		code   int
	}{
		"wrong token":  {"/sites/shop/ssl/letsencrypt", url.Values{"csrf": {"wrong"}}, true, 403},
		"no session":   {"/sites/shop/ssl/off", url.Values{"csrf": {tok}}, false, 303},
		"unknown site": {"/sites/nope/ssl/letsencrypt", url.Values{"csrf": {tok}}, true, 404},
		"domain token": {"/sites/shop/domain", url.Values{"csrf": {"wrong"}, "domain": {"example.com"}}, true, 403},
	} {
		w := do(h, "POST", tc.path, tc.form)
		if tc.cookie {
			w = do(h, "POST", tc.path, tc.form, cookie(c))
		}
		if w.Code != tc.code {
			t.Errorf("%s: %d want %d", name, w.Code, tc.code)
		}
	}
	for _, op := range []string{"site.ssl", "site.ssloff", "site.domain"} {
		if _, sent := lastOp(f, op); sent {
			t.Errorf("%s must not reach the helper", op)
		}
	}
}
