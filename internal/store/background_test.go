package store

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"mpn/internal/background"
	"mpn/internal/site"
)

func nodeSite() site.Site {
	return site.Site{Name: "api", Kind: site.Node, User: "api", Root: "/home/api/htdocs", Hostnames: []string{"api.example.com"}, NodePort: 20001, NodeVersion: "22", Entry: "server.js"}
}

func bgIn(name string) background.Input {
	return background.Input{Name: name, Args: "worker.js", Enabled: true, MemoryMB: 128}
}

func TestBgServicesAreStoredListedAndChanged(t *testing.T) {
	s, _ := open(t)
	if err := s.AddSite(nodeSite()); err != nil {
		t.Fatal(err)
	}
	id, err := s.NextBgServiceID()
	if err != nil || id != 1 {
		t.Fatalf("next id %d %v", id, err)
	}
	if err := s.AddBgService("api", site.Node, id, bgIn("bot")); err != nil {
		t.Fatal(err)
	}
	list, err := s.BgServices("api")
	if err != nil || !reflect.DeepEqual(list, []background.Service{{ID: 1, Name: "bot", Args: "worker.js", Enabled: true, MemoryMB: 128}}) {
		t.Fatalf("list %+v %v", list, err)
	}
	if err := s.UpdateBgService("api", site.Node, 1, background.Input{Name: "bot 2", Args: "x.js", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.BgServices("api"); list[0].Name != "bot 2" || list[0].Enabled || list[0].MemoryMB != 0 {
		t.Fatalf("after update %+v", list[0])
	}
	if err := s.DeleteBgService("api", 1); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.BgServices("api"); len(list) != 0 {
		t.Fatalf("after delete %+v", list)
	}
}

func TestBgServicesBelongToTheirSite(t *testing.T) {
	s, _ := open(t)
	if err := s.AddSite(nodeSite()); err != nil {
		t.Fatal(err)
	}
	if err := s.AddBgService("nope", site.Node, 1, bgIn("x")); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown site: %v", err)
	}
	if err := s.AddBgService("api", site.Node, 1, bgIn("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateBgService("other", site.Node, 1, bgIn("x")); !errors.Is(err, ErrNotFound) {
		t.Errorf("update on the wrong site: %v", err)
	}
	if err := s.DeleteBgService("other", 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete on the wrong site: %v", err)
	}
	if err := s.AddBgService("api", site.Node, 0, bgIn("y")); err == nil {
		t.Error("service number 0 must be refused")
	}
}

func TestBgServicesRefuseInvalidInputAndMaxTen(t *testing.T) {
	s, _ := open(t)
	if err := s.AddSite(nodeSite()); err != nil {
		t.Fatal(err)
	}
	bad := background.Input{Name: "x", Args: "a;rm", Enabled: true}
	if err := s.AddBgService("api", site.Node, 1, bad); err == nil {
		t.Error("an injected argument must be refused by the store too")
	}
	for i := 1; i <= background.MaxServices; i++ {
		if err := s.AddBgService("api", site.Node, int64(i), bgIn(fmt.Sprintf("s%d", i))); err != nil {
			t.Fatalf("service %d: %v", i, err)
		}
	}
	if err := s.AddBgService("api", site.Node, 99, bgIn("over")); err == nil {
		t.Error("an eleventh service must be refused")
	}
}

func TestBgServicesGoWithTheSite(t *testing.T) {
	s, _ := open(t)
	if err := s.AddSite(nodeSite()); err != nil {
		t.Fatal(err)
	}
	if err := s.AddBgService("api", site.Node, 1, bgIn("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSite("api"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM bg_services`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows left after the site was deleted: %d %v", n, err)
	}
}
