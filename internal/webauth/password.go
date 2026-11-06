// Package webauth holds what the web panel needs to know who is calling: password hashing, the stored account, and
// login sessions with their CSRF tokens. It has no HTTP code of its own.
package webauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// The cost is chosen for a 1 GB server: 32 MiB of memory for a moment on each login attempt.
const (
	argonMem       = 32 * 1024 // KiB
	argonTime      = 3
	argonThreads   = 1
	keyLen         = 32
	saltLen        = 16
	MinPasswordLen = 12
)

// hashSlot lets one hash run at a time, so a flood of login attempts cannot use more than one hash's memory.
var hashSlot = make(chan struct{}, 1)

var b64 = base64.RawStdEncoding

// Hash returns the encoded argon2id hash of pw: "argon2id$memKiB,time,threads$salt$key".
func Hash(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hashSlot <- struct{}{}
	defer func() { <-hashSlot }()
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMem, argonThreads, keyLen)
	return fmt.Sprintf("argon2id$%d,%d,%d$%s$%s", argonMem, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify reports whether pw matches the encoded hash. It takes the cost from the hash, within limits, so a damaged
// file cannot make it use gigabytes of memory.
func Verify(pw, enc string) bool {
	parts := strings.Split(enc, "$")
	if len(parts) != 4 || parts[0] != "argon2id" {
		return false
	}
	var m, t, p uint32
	if n, err := fmt.Sscanf(parts[1], "%d,%d,%d", &m, &t, &p); err != nil || n != 3 {
		return false
	}
	if m < 8*1024 || m > 128*1024 || t < 1 || t > 10 || p < 1 || p > 4 {
		return false
	}
	salt, err1 := b64.DecodeString(parts[2])
	want, err2 := b64.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(salt) < 8 || len(want) < 16 || len(want) > 64 {
		return false
	}
	hashSlot <- struct{}{}
	defer func() { <-hashSlot }()
	got := argon2.IDKey([]byte(pw), salt, t, m, uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// Account is the one login the panel knows. Only the hash is stored.
type Account struct {
	User string `json:"user"`
	Hash string `json:"hash"`
}

var (
	dummyOnce sync.Once
	dummyHash string
)

// Check reports whether user and pw are right. A wrong user name costs as much time as a wrong password, so the time
// taken does not tell a caller which user names exist.
func (a Account) Check(user, pw string) bool {
	hash := a.Hash
	if hash == "" {
		dummyOnce.Do(func() { dummyHash, _ = Hash("not a real password") })
		hash = dummyHash
	}
	passOK := Verify(pw, hash)
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.User)) == 1
	return userOK && passOK && a.Hash != ""
}

func LoadAccount(path string) (Account, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Account{}, err
	}
	var a Account
	if err := json.Unmarshal(b, &a); err != nil {
		return Account{}, fmt.Errorf("%s: %w", path, err)
	}
	if a.User == "" || a.Hash == "" {
		return Account{}, fmt.Errorf("%s: no user or hash", path)
	}
	return a, nil
}

// SaveAccount writes the account to path through a temporary file, so a reader never sees half of it. The file is
// readable by its owner only. With uid >= 0 it is given to that uid and gid; the web user must own it.
func SaveAccount(path string, a Account, uid, gid int) error {
	if a.User == "" || a.Hash == "" {
		return errors.New("account needs a user and a hash")
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".account-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && !isWindows() {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if uid >= 0 {
		if err := os.Chown(tmp.Name(), uid, gid); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), path)
}

func isWindows() bool { return os.PathSeparator == '\\' }
