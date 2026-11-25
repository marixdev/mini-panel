package cfdns

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The challenge record goes into the zone of the name, is waited for, and is removed by its own ID afterwards; nothing else is touched.
func TestTheChallengeRecordIsAddedSeenAndRemoved(t *testing.T) {
	f, c, _ := newFake(t)
	f.recs = append(f.recs, Record{ID: "keep", Type: "TXT", Name: "_acme-challenge.www.example.com", Content: "somebody else's"})
	looks := 0
	a := ACME{Client: c, Every: time.Millisecond, Lookup: func(_ context.Context, name string) ([]string, error) {
		looks++
		if name != "_acme-challenge.www.example.com" {
			t.Errorf("looked up %q", name)
		}
		if looks < 3 {
			return []string{"somebody else's"}, nil // not there yet
		}
		return []string{"somebody else's", `"v4lue"`}, nil
	}}
	cleanup, err := a.Present(context.Background(), "WWW.Example.com.", "v4lue")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.posts) != 1 || f.posts[0].Type != "TXT" || f.posts[0].Name != "_acme-challenge.www.example.com" || f.posts[0].Content != "v4lue" {
		t.Fatalf("one TXT record for the challenge: %+v", f.posts)
	}
	if looks < 3 {
		t.Error("Present must wait until the record is seen")
	}
	cleanup()
	if len(f.deleted) != 1 || f.deleted[0] == "keep" || f.bad != 0 {
		t.Errorf("only the record that was added is removed: deleted %v, bad %d", f.deleted, f.bad)
	}
	if len(f.recs) != 1 || f.recs[0].ID != "keep" {
		t.Errorf("the other record stays: %+v", f.recs)
	}
}

func TestARecordThatIsNeverSeenIsRemovedAndSaysSo(t *testing.T) {
	f, c, _ := newFake(t)
	a := ACME{Client: c, Wait: 5 * time.Millisecond, Every: time.Millisecond, Lookup: func(context.Context, string) ([]string, error) { return nil, nil }}
	_, err := a.Present(context.Background(), "example.com", "v")
	if err == nil || !strings.Contains(err.Error(), "not seen") {
		t.Fatalf("want a timeout: %v", err)
	}
	if len(f.deleted) != 1 {
		t.Error("the record is removed when the wait fails")
	}
}

func TestANameOutsideTheAccountIsNoZone(t *testing.T) {
	_, c, _ := newFake(t)
	if _, err := (ACME{Client: c}).Present(context.Background(), "other.org", "v"); err == nil {
		t.Error("a name with no zone in the account must fail")
	}
}
