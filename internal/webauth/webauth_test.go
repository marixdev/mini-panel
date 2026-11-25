package webauth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHashAndVerify(t *testing.T) {
	h, err := Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "argon2id$32768,3,1$") || strings.Contains(h, "correct") {
		t.Errorf("unexpected hash %q", h)
	}
	if !Verify("correct horse battery", h) || Verify("correct horse batterY", h) || Verify("", h) {
		t.Error("verify must accept only the right password")
	}
	h2, _ := Hash("correct horse battery")
	if h == h2 {
		t.Error("two hashes of one password must differ (random salt)")
	}
}

func TestVerifyRefusesDamagedOrDangerousHashes(t *testing.T) {
	good, _ := Hash("abcdefghijkl")
	parts := strings.Split(good, "$")
	for name, enc := range map[string]string{
		"empty":         "",
		"wrong kind":    "bcrypt$" + strings.Join(parts[1:], "$"),
		"huge memory":   "argon2id$4194304,3,1$" + parts[2] + "$" + parts[3],
		"tiny memory":   "argon2id$16,3,1$" + parts[2] + "$" + parts[3],
		"too many runs": "argon2id$32768,99,1$" + parts[2] + "$" + parts[3],
		"bad salt":      "argon2id$32768,3,1$!!$" + parts[3],
		"short key":     "argon2id$32768,3,1$" + parts[2] + "$AAAA",
		"missing part":  "argon2id$32768,3,1$" + parts[2],
	} {
		if Verify("abcdefghijkl", enc) {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAccountCheck(t *testing.T) {
	h, _ := Hash("a-long-enough-password")
	a := Account{User: "admin", Hash: h}
	if !a.Check("admin", "a-long-enough-password") {
		t.Error("right user and password must pass")
	}
	if a.Check("admin", "wrong") || a.Check("root", "a-long-enough-password") || a.Check("", "") {
		t.Error("a wrong user or password must fail")
	}
	if (Account{User: "admin"}).Check("admin", "anything") {
		t.Error("an account without a hash must never log in")
	}
}

func TestAccountFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "account.json")
	h, _ := Hash("a-long-enough-password")
	if err := SaveAccount(p, Account{User: "admin", Hash: h}, -1, -1); err != nil {
		t.Fatal(err)
	}
	got, err := LoadAccount(p)
	if err != nil || got.User != "admin" || got.Hash != h {
		t.Fatalf("%+v %v", got, err)
	}
	if !isWindows() {
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v, want 0600", fi.Mode().Perm())
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "sub", ".account-*")); len(left) != 0 {
		t.Errorf("temporary file left behind: %v", left)
	}
	if SaveAccount(p, Account{User: "admin"}, -1, -1) == nil {
		t.Error("an account without a hash must not be saved")
	}
	_ = os.WriteFile(p, []byte(`{"user":"","hash":""}`), 0o600)
	if _, err := LoadAccount(p); err == nil {
		t.Error("an empty account must not load")
	}
}

func clocked(idle, max time.Duration, limit int) (*Sessions, *time.Time) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	s := NewSessions(idle, max, limit)
	s.now = func() time.Time { return now }
	return s, &now
}

func TestSessionLifetime(t *testing.T) {
	s, now := clocked(10*time.Minute, time.Hour, 5)
	tok, sess := s.Create("admin", false)
	if len(tok) < 40 || sess.CSRF == "" || sess.CSRF == tok {
		t.Fatalf("weak tokens: %q %q", tok, sess.CSRF)
	}
	if _, ok := s.Get("not-a-token"); ok {
		t.Error("an unknown token must not log in")
	}
	for i := 0; i < 5; i++ { // active use keeps it alive past the idle time...
		*now = now.Add(8 * time.Minute)
		if _, ok := s.Get(tok); !ok {
			t.Fatalf("session ended early at step %d", i)
		}
	}
	*now = now.Add(30 * time.Minute) // ...but not past the absolute limit, and not after going idle
	if _, ok := s.Get(tok); ok {
		t.Error("session must end after the idle time and after the maximum time")
	}
	tok2, _ := s.Create("admin", false)
	*now = now.Add(11 * time.Minute)
	if _, ok := s.Get(tok2); ok {
		t.Error("an idle session must end")
	}
	tok3, _ := s.Create("admin", false)
	for i := 0; i < 9; i++ {
		*now = now.Add(9 * time.Minute)
		s.Get(tok3)
	}
	if _, ok := s.Get(tok3); ok {
		t.Error("a busy session must still end at the maximum time")
	}
}

func TestSessionLimitAndLogout(t *testing.T) {
	s, now := clocked(time.Hour, time.Hour, 2)
	a, _ := s.Create("admin", false)
	*now = now.Add(time.Second)
	b, _ := s.Create("admin", false)
	*now = now.Add(time.Second)
	s.Get(a) // a is now the more recently used one
	*now = now.Add(time.Second)
	c, _ := s.Create("admin", false)
	if _, ok := s.Get(b); ok {
		t.Error("the least recently used session should make room")
	}
	if _, ok := s.Get(a); !ok {
		t.Error("a recently used session must stay")
	}
	s.Delete(c)
	if _, ok := s.Get(c); ok {
		t.Error("a logged-out session must be gone")
	}
}

func TestCSRFCheck(t *testing.T) {
	s, _ := clocked(time.Hour, time.Hour, 5)
	tok, sess := s.Create("admin", false)
	other, otherSess := s.Create("admin", false)
	if !s.CheckCSRF(tok, sess.CSRF) {
		t.Error("the session's own token must pass")
	}
	if s.CheckCSRF(tok, otherSess.CSRF) || s.CheckCSRF(tok, "") || s.CheckCSRF("nope", sess.CSRF) || s.CheckCSRF(other, sess.CSRF) {
		t.Error("another session's token, an empty token, or an unknown session must fail")
	}
}
