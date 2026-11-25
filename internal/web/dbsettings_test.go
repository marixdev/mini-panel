package web

import (
	"net/url"
	"strings"
	"testing"
)

const dbSetInfo = `{"engine":"mariadb","settings":[{"key":"max_connections","min":10,"max":5000},{"key":"innodb_buffer_pool_size","unit":"M","min":32,"max":262144},
{"key":"slow_query_log","choices":["ON","OFF"]},{"key":"sql_mode","choices":["default","strict","relaxed"]}],
"values":{"max_connections":"300"},"current":{"max_connections":"300","innodb_buffer_pool_size":"268435456"}}`

func TestTheSettingsPageShowsWhatIsSetAndWhatRuns(t *testing.T) {
	h, _, c, _ := setup(t, byOp(map[string]string{"db.settingsinfo": dbSetInfo}))
	body := do(h, "GET", "/services/database/settings", nil, cookie(c)).Body.String()
	for _, want := range []string{`action="/services/database/settings"`, `name="max_connections" value="300"`, `name="innodb_buffer_pool_size" value=""`,
		"268435456", `<option value="relaxed">`, "khởi động lại"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestSavingSendsEveryFieldAndABadValueStaysOnThePage(t *testing.T) {
	h, f, c, tok := setup(t, byOp(map[string]string{"db.settingsinfo": dbSetInfo, "db.settings": "ok"}))
	followed(h, do(h, "POST", "/services/database/settings", url.Values{"csrf": {tok}, "max_connections": {" 500 "}, "slow_query_log": {"ON"}}, cookie(c)), c)
	req, ok := lastOp(f, "db.settings")
	if !ok || req.DBValues["max_connections"] != "500" || req.DBValues["slow_query_log"] != "ON" || req.DBValues["wait_timeout"] != "" {
		t.Errorf("sent %+v", req.DBValues)
	}
	h, f, c, tok = setup(t, byOp(map[string]string{"db.settingsinfo": dbSetInfo}))
	w := do(h, "POST", "/services/database/settings", url.Values{"csrf": {tok}, "max_connections": {"1"}}, cookie(c))
	if w.Code != 400 || !strings.Contains(w.Body.String(), `name="max_connections" value="1"`) {
		t.Errorf("%d", w.Code)
	}
	if _, ok := lastOp(f, "db.settings"); ok {
		t.Error("a value out of range is not sent")
	}
}
