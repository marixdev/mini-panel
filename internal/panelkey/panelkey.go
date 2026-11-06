// Package panelkey holds the SSH key pair the web panel uses to log in as the account of a website, for its terminal and
// its file manager. No password of any account is stored: the panel proves who it is with this key, whose public half is
// put into each site account's authorized_keys (limited to connections from this machine).
//
// The public half lives in a file only root can change. The private half belongs to the web account, so the panel can
// read it; if the web account swapped it for another key, it would only lock itself out.
package panelkey

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Marker ends the comment of the panel's line in authorized_keys. It is how an old copy of the line is found and replaced.
const Marker = "mpn-panel"

type Paths struct{ Public, Private string }

var Default = Paths{Public: "/etc/mpn/panel_ed25519.pub", Private: "/var/lib/mpn-web/ssh_ed25519"}

// consistent reports whether both files exist and form one key pair.
func consistent(p Paths) bool {
	privBytes, err := os.ReadFile(p.Private)
	if err != nil {
		return false
	}
	signer, err := ssh.ParsePrivateKey(privBytes)
	if err != nil {
		return false
	}
	pubBytes, err := os.ReadFile(p.Public)
	if err != nil {
		return false
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(pubBytes)
	return err == nil && bytes.Equal(pub.Marshal(), signer.PublicKey().Marshal())
}

// Ensure makes the key pair when it is missing or its two halves do not match, and reports whether it made a new one.
// A new pair means the old public key is no longer good anywhere, so the caller should install the new one again.
// The private file is given to uid:gid (the web account); pass -1 to leave ownership alone.
func Ensure(p Paths, uid, gid int) (created bool, err error) {
	if consistent(p) {
		return false, nil
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return false, err
	}
	block, err := ssh.MarshalPrivateKey(priv, Marker)
	if err != nil {
		return false, err
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return false, err
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + Marker + "\n"
	if err := writeFile(p.Private, pem.EncodeToMemory(block), 0o600, uid, gid); err != nil {
		return false, err
	}
	if err := writeFile(p.Public, []byte(line), 0o644, -1, -1); err != nil {
		return false, err
	}
	return true, nil
}

// writeFile writes through a new file that is renamed into place. The directory may belong to the web account, so the
// file is made with O_EXCL (it never follows a link someone planted) and changed through its descriptor, not its name.
func writeFile(path string, data []byte, mode os.FileMode, uid, gid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	rnd := make([]byte, 6)
	_, _ = rand.Read(rnd)
	tmp := filepath.Join(filepath.Dir(path), ".key-"+hex.EncodeToString(rnd))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if uid >= 0 {
		if err := f.Chown(uid, gid); err != nil {
			f.Close()
			return err
		}
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil && os.PathSeparator != '\\' {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// AuthorizedLine returns the line to put in a site account's authorized_keys. The key can be used only from this
// machine, cannot forward anything, and still gets a terminal (the web terminal needs one). The public file is read
// back and checked to be one plain ed25519 key, so a file that was swapped for something else is refused and nothing
// but a public key can ever be copied into an account.
func AuthorizedLine(publicPath string) (string, error) {
	b, err := os.ReadFile(publicPath)
	if err != nil {
		return "", err
	}
	if len(b) > 1024 {
		return "", errors.New("the panel's public key file is too large")
	}
	pub, _, options, rest, err := ssh.ParseAuthorizedKey(b)
	if err != nil || pub.Type() != ssh.KeyAlgoED25519 || len(options) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return "", fmt.Errorf("%s does not hold exactly one plain ed25519 public key", publicPath)
	}
	key := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	return `restrict,pty,from="127.0.0.1,::1" ` + key + " " + Marker, nil
}
