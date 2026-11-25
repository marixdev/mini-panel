package webauth

import (
	"strings"
	"testing"
	"time"
)

// The secret and the codes are the test vectors of RFC 6238 (the 6-digit tail of the 8-digit values it lists).
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // "12345678901234567890"

func TestTOTPMatchesTheRFCVectors(t *testing.T) {
	for _, c := range []struct {
		unix int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		got, err := totpCode(rfcSecret, c.unix/30)
		if err != nil || got != c.want {
			t.Errorf("t=%d: %q %v, want %q", c.unix, got, err, c.want)
		}
	}
}

func TestACodeIsAcceptedOnceAndOnlyNearItsTime(t *testing.T) {
	now := time.Unix(1111111109, 0)
	step, ok := VerifyTOTP(rfcSecret, "081804", now, 0)
	if !ok || step != 1111111109/30 {
		t.Fatalf("right code: %d %v", step, ok)
	}
	if _, ok := VerifyTOTP(rfcSecret, "081 804", now, 0); !ok {
		t.Error("spaces in the code are ignored")
	}
	if _, ok := VerifyTOTP(rfcSecret, "081804", now, step); ok {
		t.Error("a code that was used must not work again")
	}
	if _, ok := VerifyTOTP(rfcSecret, "081804", now.Add(2*time.Minute), 0); ok {
		t.Error("a code from two minutes ago is too old")
	}
	prev, _ := totpCode(rfcSecret, step-1)
	if _, ok := VerifyTOTP(rfcSecret, prev, now, 0); !ok {
		t.Error("the step before is accepted: clocks differ a little")
	}
	for _, bad := range []string{"", "12345", "1234567", "12345a", "０８１８０４", "081804 "} {
		if bad != "081804 " { // a trailing space is ignored like any other
			if _, ok := VerifyTOTP(rfcSecret, bad, now, 0); ok {
				t.Errorf("%q must be refused", bad)
			}
		}
	}
	if _, ok := VerifyTOTP("not base32!", "081804", now, 0); ok {
		t.Error("a damaged secret verifies nothing")
	}
}

func TestSecretsAreRandomAndWellFormed(t *testing.T) {
	a, err := NewTOTPSecret()
	b, _ := NewTOTPSecret()
	if err != nil || a == b || len(a) != 32 || strings.Trim(a, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567") != "" {
		t.Errorf("%q %q", a, b)
	}
	if _, err := totpCode(a, 1); err != nil {
		t.Error(err)
	}
	if g := GroupSecret("ABCDEFGHIJ"); g != "ABCD EFGH IJ" {
		t.Errorf("group: %q", g)
	}
}

func TestTheAppAddressCarriesTheSecretAndTheSettings(t *testing.T) {
	u := TOTPURI("Mini Panel", "admin", "ABCDEFGH")
	for _, want := range []string{"otpauth://totp/Mini%20Panel:admin?", "secret=ABCDEFGH", "issuer=Mini+Panel", "algorithm=SHA1", "digits=6", "period=30"} {
		if !strings.Contains(u, want) {
			t.Errorf("missing %q in %s", want, u)
		}
	}
}

func TestEndingTheOtherSessionsKeepsThisOne(t *testing.T) {
	s := NewSessions(time.Hour, time.Hour, 10)
	a, _ := s.Create("admin", false)
	b, _ := s.Create("admin", false)
	s.DeleteExcept(a)
	if _, ok := s.Get(a); !ok {
		t.Error("this session must stay")
	}
	if _, ok := s.Get(b); ok {
		t.Error("the other session must end")
	}
}
