// Package acme gets certificates from Let's Encrypt with the HTTP-01 challenge.
package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	xacme "golang.org/x/crypto/acme"
)

const (
	Production = "https://acme-v02.api.letsencrypt.org/directory"
	// Staging issues certificates browsers do not trust, with far higher rate limits. Use it for trial runs.
	Staging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

type Config struct {
	Directory  string // Production or Staging
	Email      string // optional contact address
	AccountKey string // file holding the account key; created on first use
	WebRoot    string // directory nginx serves under /.well-known/acme-challenge/
}

var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

// Issue proves control of every name over HTTP and returns the certificate chain and a new private key, both PEM.
func Issue(ctx context.Context, c Config, names []string) (chainPEM, keyPEM []byte, err error) {
	if len(names) == 0 {
		return nil, nil, errors.New("no host names given")
	}
	accountKey, err := loadOrCreateKey(c.AccountKey)
	if err != nil {
		return nil, nil, err
	}
	client := &xacme.Client{Key: accountKey, DirectoryURL: c.Directory, UserAgent: "mpn"}
	acct := &xacme.Account{}
	if c.Email != "" {
		acct.Contact = []string{"mailto:" + c.Email}
	}
	if _, err := client.Register(ctx, acct, xacme.AcceptTOS); err != nil && !errors.Is(err, xacme.ErrAccountAlreadyExists) {
		return nil, nil, fmt.Errorf("registering account: %w", err)
	}
	order, err := client.AuthorizeOrder(ctx, xacme.DomainIDs(names...))
	if err != nil {
		return nil, nil, fmt.Errorf("creating order: %w", err)
	}
	for _, u := range order.AuthzURLs {
		if err := solve(ctx, client, c.WebRoot, u); err != nil {
			return nil, nil, err
		}
	}
	if order, err = client.WaitOrder(ctx, order.URI); err != nil {
		return nil, nil, fmt.Errorf("waiting for order: %w", err)
	}
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: names[0]}, DNSNames: names}, certKey)
	if err != nil {
		return nil, nil, err
	}
	der, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, nil, fmt.Errorf("finalizing order: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(certKey)
	if err != nil {
		return nil, nil, err
	}
	return encodeChain(der), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

func solve(ctx context.Context, client *xacme.Client, webRoot, authzURL string) error {
	z, err := client.GetAuthorization(ctx, authzURL)
	if err != nil {
		return err
	}
	if z.Status == xacme.StatusValid {
		return nil
	}
	var chal *xacme.Challenge
	for _, ch := range z.Challenges {
		if ch.Type == "http-01" {
			chal = ch
		}
	}
	if chal == nil {
		return fmt.Errorf("%s: the CA offered no http-01 challenge", z.Identifier.Value)
	}
	body, err := client.HTTP01ChallengeResponse(chal.Token)
	if err != nil {
		return err
	}
	cleanup, err := putChallenge(webRoot, chal.Token, body)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := client.Accept(ctx, chal); err != nil {
		return err
	}
	if _, err := client.WaitAuthorization(ctx, z.URI); err != nil {
		return fmt.Errorf("%s: %w", z.Identifier.Value, err)
	}
	return nil
}

// putChallenge writes the key authorization where nginx serves it and returns a function that removes it.
// The token comes from the CA, so it is checked before it becomes part of a file name.
func putChallenge(webRoot, token, body string) (func(), error) {
	if !tokenRe.MatchString(token) {
		return nil, fmt.Errorf("unexpected challenge token %q", token)
	}
	dir := filepath.Join(webRoot, ".well-known", "acme-challenge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, token)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		return nil, err
	}
	return func() { os.Remove(p) }, nil
}

func loadOrCreateKey(path string) (*ecdsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(b)
		if block == nil {
			return nil, fmt.Errorf("%s holds no PEM data", path)
		}
		return x509.ParseECPrivateKey(block.Bytes)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func encodeChain(der [][]byte) []byte {
	var out []byte
	for _, d := range der {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: d})...)
	}
	return out
}
