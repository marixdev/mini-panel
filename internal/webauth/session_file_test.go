package webauth

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A restart of the panel (an update) must not sign anybody out: the sessions are read back from the file, and the file holds no
// cookie value, only its hash.
func TestSessionsOutliveARestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sessions.json")
	s, now := clocked(2*time.Hour, 24*time.Hour, 10)
	s.File = file
	tok, sess := s.Create("admin", false)
	b, err := os.ReadFile(file)
	if err != nil || strings.Contains(string(b), tok) {
		t.Fatalf("the file must exist and not hold the cookie value: %v\n%s", err, b)
	}
	if fi, _ := os.Stat(file); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("the file must be private: %v", fi.Mode())
	}
	again := NewSessions(2*time.Hour, 24*time.Hour, 10)
	again.now = func() time.Time { return *now }
	again.File = file
	again.Load()
	got, ok := again.Get(tok)
	if !ok || got.User != "admin" || got.CSRF != sess.CSRF {
		t.Fatalf("the session must come back after a restart: %+v %v", got, ok)
	}
	again.Delete(tok)
	third := NewSessions(2*time.Hour, 24*time.Hour, 10)
	third.File = file
	third.now = func() time.Time { return *now }
	third.Load()
	if _, ok := third.Get(tok); ok {
		t.Error("a signed-out session must stay signed out after a restart")
	}
}

func TestARememberedBrowserLastsLonger(t *testing.T) {
	s, now := clocked(2*time.Hour, 24*time.Hour, 10)
	s.LongIdle, s.LongMax = 30*24*time.Hour, 30*24*time.Hour
	short, a := s.Create("admin", false)
	long, b := s.Create("admin", true)
	if s.Lifetime(a) != 0 || s.Lifetime(b) != 30*24*time.Hour {
		t.Errorf("only a remembered browser gets a lasting cookie: %v %v", s.Lifetime(a), s.Lifetime(b))
	}
	*now = now.Add(3 * time.Hour)
	if _, ok := s.Get(short); ok {
		t.Error("an ordinary session ends after 2 hours without use")
	}
	*now = now.Add(10 * 24 * time.Hour)
	if _, ok := s.Get(long); !ok {
		t.Error("a remembered browser is still signed in after 10 days without use")
	}
	*now = now.Add(21 * 24 * time.Hour)
	if _, ok := s.Get(long); ok {
		t.Error("a remembered browser ends after 30 days")
	}
}

func TestADamagedSessionsFileSignsEverybodyOut(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sessions.json")
	if err := os.WriteFile(file, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSessions(time.Hour, time.Hour, 10)
	s.File = file
	s.Load()
	if s.Count() != 0 {
		t.Error("nothing is read from a damaged file")
	}
}
