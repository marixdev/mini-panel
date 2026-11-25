package tune

import (
	"strings"
	"testing"
)

func TestOnlyKnownSettingsInTheirRangeAreTaken(t *testing.T) {
	got, err := CheckDBValues(map[string]string{"max_connections": " 300 ", "innodb_buffer_pool_size": "512M", "slow_query_log": "on", "wait_timeout": "", "sql_mode": "default"})
	if err != nil {
		t.Fatal(err)
	}
	if got["max_connections"] != "300" || got["innodb_buffer_pool_size"] != "512" || got["slow_query_log"] != "ON" || len(got) != 3 {
		t.Errorf("%v", got)
	}
	for _, bad := range []map[string]string{
		{"max_connections": "5"}, {"max_connections": "abc"}, {"innodb_buffer_pool_size": "1G"}, {"slow_query_log": "maybe"},
		{"datadir": "/tmp"}, {"max_connections": "100\nskip-grant-tables"}, {"init_file": "/etc/shadow"},
	} {
		if _, err := CheckDBValues(bad); err == nil {
			t.Errorf("%v must be refused", bad)
		}
	}
}

func TestTheFileIsWrittenInOrderAndReadBack(t *testing.T) {
	in := map[string]string{"sql_mode": "relaxed", "max_connections": "300", "innodb_buffer_pool_size": "512", "long_query_time": "2"}
	text := UserDBConf(in)
	want := "[mysqld]\nmax_connections = 300\ninnodb_buffer_pool_size = 512M\nlong_query_time = 2\nsql_mode = \"NO_ENGINE_SUBSTITUTION\"\n"
	if !strings.HasSuffix(text, want) {
		t.Errorf("got:\n%s\nwant ending:\n%s", text, want)
	}
	back := ParseUserDBConf(text)
	for k, v := range in {
		if back[k] != v {
			t.Errorf("%s: read back %q, want %q", k, back[k], v)
		}
	}
}
