// Package panelssh is how the web panel logs in as the account of a website: over SSH to this very machine, with the
// key of package panelkey. The terminal and the file manager both run on such a connection, so they can do exactly
// what that account can do, no more.
package panelssh

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	Addr        string        // the SSH server of this machine, such as 127.0.0.1:22
	KeyPath     string        // the panel's private key
	HostKeyGlob string        // the server's public host keys, such as /etc/ssh/ssh_host_*_key.pub
	Timeout     time.Duration // for connecting and logging in
}

var userRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// loadHostKeys reads the public host keys of this machine. The connection is accepted only if the server presents one
// of them, so something else answering on the port cannot pose as the machine's SSH server.
func loadHostKeys(glob string) ([]ssh.PublicKey, []string, error) {
	files, err := filepath.Glob(glob)
	if err != nil {
		return nil, nil, err
	}
	var keys []ssh.PublicKey
	var algos []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		k, _, _, _, err := ssh.ParseAuthorizedKey(b)
		if err != nil {
			continue
		}
		keys = append(keys, k)
		if k.Type() == ssh.KeyAlgoRSA {
			algos = append(algos, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256)
		} else {
			algos = append(algos, k.Type())
		}
	}
	if len(keys) == 0 {
		return nil, nil, fmt.Errorf("no SSH host key found at %s", glob)
	}
	return keys, algos, nil
}

// Dial logs in as user and returns the connection. The caller closes it.
func (c Client) Dial(user string) (*ssh.Client, error) {
	if !userRe.MatchString(user) {
		return nil, fmt.Errorf("invalid account name %q", user)
	}
	keyBytes, err := os.ReadFile(c.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("the panel's SSH key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("the panel's SSH key: %w", err)
	}
	hostKeys, algos, err := loadHostKeys(c.HostKeyGlob)
	if err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			for _, k := range hostKeys {
				if bytes.Equal(k.Marshal(), key.Marshal()) {
					return nil
				}
			}
			return errors.New("the SSH server does not present a host key of this machine")
		},
		HostKeyAlgorithms: algos,
		Timeout:           timeout,
	}
	return ssh.Dial("tcp", c.Addr, cfg)
}
