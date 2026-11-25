package ctl

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBgOperationsAreFixedCommandsAndArgumentsTravelOnStandardInput(t *testing.T) {
	if IsWrite("site.bglist") || !IsWrite("site.bgadd") || !IsWrite("site.bgset") || !IsWrite("site.bgdel") || !IsWrite("site.bgrestart") {
		t.Fatal("reads and writes are mixed up")
	}
	got, _ := Request{Op: "site.bglist", Site: "shop"}.Argv()
	if !reflect.DeepEqual(got, []string{"bg", "list", "-site", "shop", "-json"}) {
		t.Errorf("%v", got)
	}
	r := Request{Op: "site.bgadd", Site: "shop", BgName: "queue", BgArgs: "artisan queue:work --token=SECRET", BgEnabled: true, BgMemoryMB: 256}
	argv, err := r.Argv()
	if err != nil || !reflect.DeepEqual(argv, []string{"bg", "add", "-site", "shop", "-stdin", "-apply"}) {
		t.Fatalf("%v %v", argv, err)
	}
	for _, a := range argv {
		if strings.Contains(a, "SECRET") {
			t.Error("the arguments must not be in argv: they can hold a secret")
		}
	}
	var in struct {
		Name     string `json:"name"`
		Args     string `json:"args"`
		Enabled  bool   `json:"enabled"`
		MemoryMB int    `json:"memory_mb"`
	}
	if err := json.Unmarshal([]byte(r.Stdin()), &in); err != nil || in.Name != "queue" || !strings.Contains(in.Args, "SECRET") || in.MemoryMB != 256 {
		t.Errorf("stdin %q %v", r.Stdin(), err)
	}
	if (Request{Op: "site.bglist", Site: "shop", BgArgs: "x"}).Stdin() != "" {
		t.Error("only add and set send text")
	}
	got, _ = Request{Op: "site.bgset", Site: "shop", BgID: 3, BgName: "q", BgArgs: "w.js", BgEnabled: true}.Argv()
	if !reflect.DeepEqual(got, []string{"bg", "set", "-site", "shop", "-id", "3", "-stdin", "-apply"}) {
		t.Errorf("%v", got)
	}
	got, _ = Request{Op: "site.bgdel", Site: "shop", BgID: 3}.Argv()
	if !reflect.DeepEqual(got, []string{"bg", "remove", "-site", "shop", "-id", "3", "-apply"}) {
		t.Errorf("%v", got)
	}
	got, _ = Request{Op: "site.bgrestart", Site: "shop", BgID: 3}.Argv()
	if !reflect.DeepEqual(got, []string{"bg", "restart", "-site", "shop", "-id", "3", "-apply"}) {
		t.Errorf("%v", got)
	}
}

func TestABackgroundRequestThatIsNotValidIsRefusedBeforeAnythingRuns(t *testing.T) {
	for name, r := range map[string]Request{
		"a bad site":           {Op: "site.bgadd", Site: "../x", BgName: "q", BgArgs: "w.js", BgEnabled: true},
		"a bad name":           {Op: "site.bgadd", Site: "shop", BgName: "a;b", BgArgs: "w.js", BgEnabled: true},
		"an injected argument": {Op: "site.bgadd", Site: "shop", BgName: "q", BgArgs: "w.js;rm -rf /", BgEnabled: true},
		"a substitution":       {Op: "site.bgset", Site: "shop", BgID: 2, BgName: "q", BgArgs: "$(id)"},
		"a second line":        {Op: "site.bgadd", Site: "shop", BgName: "q", BgArgs: "w.js\n* * * * * root id", BgEnabled: true},
		"an absolute path":     {Op: "site.bgadd", Site: "shop", BgName: "q", BgArgs: "/usr/bin/x", BgEnabled: true},
		"a dot dot path":       {Op: "site.bgadd", Site: "shop", BgName: "q", BgArgs: "../x.js", BgEnabled: true},
		"memory too small":     {Op: "site.bgadd", Site: "shop", BgName: "q", BgArgs: "w.js", BgMemoryMB: 1},
		"service zero":         {Op: "site.bgdel", Site: "shop", BgID: 0},
		"negative service":     {Op: "site.bgrestart", Site: "shop", BgID: -3},
		"service zero on set":  {Op: "site.bgset", Site: "shop", BgID: 0, BgName: "q", BgArgs: "w.js"},
	} {
		if _, err := r.Argv(); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}
