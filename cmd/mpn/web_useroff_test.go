package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mpn/internal/webauth"
)

func TestDisableDelegatedSetsDisabledAndEndsSessions(t *testing.T) {
	dir := t.TempDir()
	acctPath := filepath.Join(dir, "account.json")
	if err := webauth.SaveAccount(acctPath, webauth.Account{User: "admin", Hash: mustHash(t, "admin-password-123")}, -1, -1); err != nil {
		t.Fatal(err)
	}
	usersPath := filepath.Join(dir, "users.json")
	ann := webauth.User{Name: "ann", Hash: mustHash(t, "ann-password-123"), Sites: map[string]webauth.Role{"blog": webauth.RoleView}}
	bob := webauth.User{Name: "bob", Hash: mustHash(t, "bob-password-123"), Sites: map[string]webauth.Role{"blog": webauth.RoleView}}
	if err := webauth.SaveUsers(usersPath, []webauth.User{ann, bob}, "admin", -1, -1); err != nil {
		t.Fatal(err)
	}
	sessPath := filepath.Join(dir, "sessions.json")
	sess := webauth.NewSessions(time.Hour, time.Hour, 10)
	sess.File = sessPath
	annTok, _ := sess.Create("ann", false)
	sess.Create("bob", false)

	msg, err := disableDelegated(acctPath, "ann", -1, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "disabled") {
		t.Errorf("message %q does not say disabled", msg)
	}
	users, err := webauth.LoadUsers(usersPath, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u.Name == "ann" && !u.Disabled {
			t.Error("ann was not disabled")
		}
		if u.Name == "bob" && u.Disabled {
			t.Error("bob was disabled too")
		}
	}
	// A fresh session store loaded from the file: ann's session is gone, bob's is kept.
	check := webauth.NewSessions(time.Hour, time.Hour, 10)
	check.File = sessPath
	check.Load()
	if _, ok := check.Get(annTok); ok {
		t.Error("ann's session survived the disable")
	}
	if check.Count() != 1 {
		t.Errorf("sessions left = %d, want 1 (bob)", check.Count())
	}
}

func TestDisableDelegatedRefusesUnknownAndAlreadyDisabled(t *testing.T) {
	dir := t.TempDir()
	acctPath := filepath.Join(dir, "account.json")
	if err := webauth.SaveAccount(acctPath, webauth.Account{User: "admin", Hash: mustHash(t, "admin-password-123")}, -1, -1); err != nil {
		t.Fatal(err)
	}
	if _, err := disableDelegated(acctPath, "nobody", -1, -1); err == nil {
		t.Error("unknown user: no error")
	}
	if _, err := os.Stat(filepath.Join(dir, "users.json")); err == nil {
		t.Error("users.json written for an unknown user")
	}
	usersPath := filepath.Join(dir, "users.json")
	ann := webauth.User{Name: "ann", Hash: mustHash(t, "ann-password-123"), Disabled: true, Sites: map[string]webauth.Role{}}
	if err := webauth.SaveUsers(usersPath, []webauth.User{ann}, "admin", -1, -1); err != nil {
		t.Fatal(err)
	}
	msg, err := disableDelegated(acctPath, "ann", -1, -1)
	if err != nil || !strings.Contains(msg, "already") {
		t.Errorf("already disabled: msg=%q err=%v", msg, err)
	}
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := webauth.Hash(pw)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
