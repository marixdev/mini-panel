package webauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Two-step sign-in with a code from an authenticator app (RFC 6238: HMAC-SHA1, 30 seconds, 6 digits, the settings every
// app understands). The secret is kept in the account file next to the password hash, readable by the panel's own account
// only. Nothing here talks to the network.

const (
	totpPeriod = 30
	totpSkew   = 1 // a code from the step before or after is accepted: clocks differ by a few seconds
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a new random secret, 160 bits as the RFC advises, written in base32 the way apps ask for it.
func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

func totpCode(secret string, step int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil || len(key) < 10 {
		return "", errors.New("the secret is not valid")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := (uint32(sum[off]&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])) % 1_000_000
	return fmt.Sprintf("%06d", v), nil
}

// CodeAt is the code an authenticator app shows at time t. The panel never needs it to sign anybody in; its tests use it to play
// the part of the app.
func CodeAt(secret string, t time.Time) (string, error) { return totpCode(secret, t.Unix()/totpPeriod) }

// VerifyTOTP reports whether code is right for the secret at the time now, and for which step. A step that is not after last is
// refused, so a code that was used once cannot be used again within its 30 seconds (or by somebody who watched it being typed).
// Spaces in the code are ignored, because apps show it as "123 456".
func VerifyTOTP(secret, code string, now time.Time, last int64) (step int64, ok bool) {
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != 6 {
		return 0, false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	cur := now.Unix() / totpPeriod
	for d := int64(-totpSkew); d <= totpSkew; d++ {
		st := cur + d
		if st <= last {
			continue
		}
		want, err := totpCode(secret, st)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return st, true
		}
	}
	return 0, false
}

// TOTPURI is the address an authenticator app reads from a QR code to add the account.
func TOTPURI(issuer, account, secret string) string {
	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?secret=" + secret +
		"&issuer=" + url.QueryEscape(issuer) + "&algorithm=SHA1&digits=6&period=30"
}

// GroupSecret writes a secret in groups of four for a person to type: "ABCD EFGH ...".
func GroupSecret(s string) string {
	var out []string
	for len(s) > 4 {
		out, s = append(out, s[:4]), s[4:]
	}
	return strings.Join(append(out, s), " ")
}
