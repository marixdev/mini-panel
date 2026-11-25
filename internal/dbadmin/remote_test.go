package dbadmin

import (
	"strings"
	"testing"
)

func TestHostsAsTheyAreTyped(t *testing.T) {
	for in, want := range map[string]string{"203.0.113.5": "203.0.113.5", " 203.0.113.0/24 ": "203.0.113.0/24", "203.0.113.9/24": "203.0.113.0/24",
		"%": "%", "2001:db8::1": "2001:db8::1", "198.51.100.7/32": "198.51.100.7"} {
		if got, ok := NormalizeHost(in); !ok || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "example.com", "1.2.3", "10.0.0.0/4", "2001:db8::/64", "1.2.3.4'; DROP USER root; --", "localhost", "%.example.com"} {
		if _, ok := NormalizeHost(bad); ok {
			t.Errorf("%q must be refused", bad)
		}
	}
	got, err := ParseHosts("203.0.113.5, 203.0.113.5\n198.51.100.0/24;  %")
	if err != nil || strings.Join(got, " ") != "203.0.113.5 198.51.100.0/24 %" {
		t.Errorf("%v %v", got, err)
	}
	if _, err := ParseHosts("1.2.3.4 nope"); err == nil {
		t.Error("one bad address refuses the list")
	}
}

// hostFake answers the queries of remote access as a server with the given remote hosts would.
type hostFake struct {
	fake
	hosts []string
	shown string
}

func (f *hostFake) Run(sql string) (string, error) {
	f.sqls = append(f.sqls, sql)
	switch {
	case strings.Contains(sql, "SELECT host FROM mysql.user"):
		return strings.Join(f.hosts, "\n") + "\n", nil
	case strings.Contains(sql, "SHOW CREATE USER"):
		return f.shown + "\n", nil
	case strings.Contains(sql, "information_schema"):
		return "1\n", nil
	}
	return "", nil
}

func TestRemoteUsersCopyTheLocalPasswordAndOnlyChangeWhatDiffers(t *testing.T) {
	f := &hostFake{hosts: []string{"198.51.100.0/255.255.255.0", "203.0.113.5"},
		shown: "CREATE USER `blog`@`localhost` IDENTIFIED BY PASSWORD '*ABCDEF0123'"}
	a := Admin{f}
	if hs, _ := a.Hosts("blog"); strings.Join(hs, " ") != "198.51.100.0/24 203.0.113.5" {
		t.Fatalf("hosts are read back as typed: %v", hs)
	}
	if err := a.SetHosts("blog", []string{"203.0.113.5", "%"}, false); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(f.sqls, "")
	for _, want := range []string{"DROP USER IF EXISTS 'blog'@'198.51.100.0/255.255.255.0';", "CREATE USER 'blog'@'%' IDENTIFIED BY PASSWORD '*ABCDEF0123';",
		"GRANT ALL PRIVILEGES ON `blog`.* TO 'blog'@'%';"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if strings.Contains(all, "'blog'@'203.0.113.5'") || strings.Contains(all, "'blog'@'localhost';") && strings.Contains(all, "DROP USER IF EXISTS 'blog'@'localhost'") {
		t.Errorf("a host that stays, and the local user, are not touched:\n%s", all)
	}
}

func TestANetworkIsWrittenWithItsMaskAndMySQLIsAskedForHex(t *testing.T) {
	f := &hostFake{shown: "CREATE USER 'shop'@'localhost' IDENTIFIED WITH 'caching_sha2_password' AS 0x24412430 REQUIRE NONE"}
	if err := (Admin{f}).SetHosts("shop", []string{"203.0.113.0/24"}, true); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(f.sqls, "")
	if !strings.Contains(all, "print_identified_with_as_hex = ON") ||
		!strings.Contains(all, "CREATE USER 'shop'@'203.0.113.0/255.255.255.0' IDENTIFIED WITH 'caching_sha2_password' AS 0x24412430 REQUIRE NONE;") {
		t.Errorf("%s", all)
	}
}

func TestBadInputNeverReachesTheServer(t *testing.T) {
	f := &hostFake{}
	if err := (Admin{f}).SetHosts("blog", []string{"1.2.3.4'; DROP DATABASE mysql; --"}, false); err == nil {
		t.Error("a host that is not an address must be refused")
	}
	if err := (Admin{f}).SetHosts("blog", []string{"203.0.113.9/24"}, false); err == nil {
		t.Error("only the normalized form is taken")
	}
	if len(f.sqls) != 0 {
		t.Errorf("nothing is sent: %v", f.sqls)
	}
	if _, err := createLikeSQL("CREATE USER `other`@`localhost` IDENTIFIED BY PASSWORD 'x'", "blog", "%"); err == nil {
		t.Error("the statement of another user is not copied")
	}
}

func TestPasswordAndDropReachTheRemoteUsers(t *testing.T) {
	f := &hostFake{hosts: []string{"203.0.113.5"}}
	if err := (Admin{f}).SetPassword("blog", "Pw123456789xyz"); err != nil {
		t.Fatal(err)
	}
	if err := (Admin{f}).Drop("blog"); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(f.sqls, "")
	if !strings.Contains(all, "ALTER USER 'blog'@'203.0.113.5' IDENTIFIED BY 'Pw123456789xyz';") || !strings.Contains(all, "DROP USER IF EXISTS 'blog'@'203.0.113.5';") {
		t.Errorf("%s", all)
	}
}
