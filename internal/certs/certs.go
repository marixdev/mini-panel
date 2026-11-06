// Package certs reads and checks the certificates a person brings (a bought certificate, a Cloudflare origin certificate)
// and makes the requests for new ones. It never touches the disk: callers pass PEM text and get PEM text back.
package certs

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Problems a caller may want to tell apart from others.
var (
	ErrEncryptedKey = errors.New("the private key is protected by a passphrase")
	ErrUntrusted    = errors.New("the certificate does not chain to a trusted authority")
)

// Result is a checked certificate, ready to be installed.
type Result struct {
	Fullchain []byte    // the certificate followed by its intermediates, the order nginx wants
	Key       []byte    // the private key as PKCS#8 PEM
	Issuer    string    // who issued it
	Names     []string  // the names it covers
	NotBefore time.Time // validity
	NotAfter  time.Time
	Trusted   bool // it chains to an authority in the roots given to Check
}

// Info describes a certificate that is installed already.
type Info struct {
	Issuer, Subject     string
	Names               []string
	NotBefore, NotAfter time.Time
}

func blocks(data []byte) []*pem.Block {
	var out []*pem.Block
	for {
		var b *pem.Block
		b, data = pem.Decode(data)
		if b == nil {
			return out
		}
		out = append(out, b)
	}
}

// ParseKey reads one private key in PEM form. A key with a passphrase is refused, because nginx cannot start with one.
func ParseKey(data []byte) (crypto.Signer, error) {
	for _, b := range blocks(data) {
		if strings.Contains(b.Type, "ENCRYPTED") || b.Headers["Proc-Type"] != "" {
			return nil, ErrEncryptedKey
		}
		var k any
		var err error
		switch b.Type {
		case "RSA PRIVATE KEY":
			k, err = x509.ParsePKCS1PrivateKey(b.Bytes)
		case "EC PRIVATE KEY":
			k, err = x509.ParseECPrivateKey(b.Bytes)
		case "PRIVATE KEY":
			k, err = x509.ParsePKCS8PrivateKey(b.Bytes)
		default:
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("the private key cannot be read: %w", err)
		}
		s, ok := k.(crypto.Signer)
		if !ok {
			return nil, errors.New("this kind of private key is not supported")
		}
		switch pub := s.Public().(type) {
		case *rsa.PublicKey:
			if pub.N.BitLen() < 2048 {
				return nil, fmt.Errorf("the RSA key has %d bits; at least 2048 are needed", pub.N.BitLen())
			}
		case *ecdsa.PublicKey, ed25519.PublicKey:
		default:
			return nil, errors.New("this kind of private key is not supported")
		}
		return s, nil
	}
	return nil, errors.New("no private key found; paste the text that starts with -----BEGIN PRIVATE KEY-----")
}

// certificates reads every certificate in the PEM text.
func certificates(data []byte) ([]*x509.Certificate, error) {
	var out []*x509.Certificate
	for _, b := range blocks(data) {
		if b.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, fmt.Errorf("a certificate cannot be read: %w", err)
		}
		out = append(out, c)
	}
	return out, nil
}

func samePublicKey(a, b crypto.PublicKey) bool {
	ab, err1 := x509.MarshalPKIXPublicKey(a)
	bb, err2 := x509.MarshalPKIXPublicKey(b)
	return err1 == nil && err2 == nil && bytes.Equal(ab, bb)
}

func pemOf(c *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
}

// namesOf lists what a certificate covers.
func namesOf(c *x509.Certificate) []string {
	names := append([]string{}, c.DNSNames...)
	if len(names) == 0 && c.Subject.CommonName != "" {
		names = append(names, c.Subject.CommonName)
	}
	return names
}

// Check validates the key, the certificate and the chain a person pasted, against the names the site must be served under.
// certPEM may hold the whole bundle; chainPEM is optional. The leaf is the certificate that belongs to the key, whatever the order.
// roots are the authorities to trust; pass nil for the system's. A certificate that is valid but not trusted is returned with
// Trusted false and ErrUntrusted, so the caller can let the person install it anyway (an origin certificate behind Cloudflare is such a case).
func Check(keyPEM, certPEM, chainPEM []byte, names []string, now time.Time, roots *x509.CertPool) (*Result, error) {
	key, err := ParseKey(keyPEM)
	if err != nil {
		return nil, err
	}
	all, err := certificates(append(append([]byte{}, certPEM...), append([]byte("\n"), chainPEM...)...))
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, errors.New("no certificate found; paste the text that starts with -----BEGIN CERTIFICATE-----")
	}
	var leaf *x509.Certificate
	var rest []*x509.Certificate
	for _, c := range all {
		if leaf == nil && samePublicKey(c.PublicKey, key.Public()) {
			leaf = c
		} else {
			rest = append(rest, c)
		}
	}
	if leaf == nil {
		return nil, errors.New("the private key does not belong to this certificate")
	}
	if now.Before(leaf.NotBefore) {
		return nil, fmt.Errorf("the certificate is valid only from %s", leaf.NotBefore.Format("02/01/2006"))
	}
	if now.After(leaf.NotAfter) {
		return nil, fmt.Errorf("the certificate expired on %s", leaf.NotAfter.Format("02/01/2006"))
	}
	for _, n := range names {
		if err := leaf.VerifyHostname(n); err != nil {
			return nil, fmt.Errorf("the certificate does not cover %s (it covers: %s)", n, strings.Join(namesOf(leaf), ", "))
		}
	}

	// Order the chain by following issuers from the leaf, and drop what does not belong (a root, a stray certificate).
	inter := x509.NewCertPool()
	for _, c := range rest {
		inter.AddCert(c)
	}
	chain, cur := []*x509.Certificate{}, leaf
	for i := 0; i < 8; i++ {
		var next *x509.Certificate
		for _, c := range rest {
			if bytes.Equal(c.RawSubject, cur.RawIssuer) && !bytes.Equal(c.Raw, cur.Raw) && cur.CheckSignatureFrom(c) == nil {
				next = c
				break
			}
		}
		if next == nil || bytes.Equal(next.RawSubject, next.RawIssuer) { // a root is not sent
			break
		}
		chain, cur = append(chain, next), next
	}
	res := &Result{Issuer: leaf.Issuer.CommonName, Names: namesOf(leaf), NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter}
	if res.Issuer == "" {
		res.Issuer = leaf.Issuer.String()
	}
	res.Fullchain = pemOf(leaf)
	for _, c := range chain {
		res.Fullchain = append(res.Fullchain, pemOf(c)...)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	res.Key = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter, CurrentTime: now}); err == nil {
		res.Trusted = true
		return res, nil
	}
	return res, ErrUntrusted
}

// Describe reads the certificate at the start of a PEM chain.
func Describe(fullchain []byte) (*Info, error) {
	all, err := certificates(fullchain)
	if err != nil || len(all) == 0 {
		return nil, errors.New("no certificate in the file")
	}
	c := all[0]
	issuer := c.Issuer.CommonName
	if issuer == "" {
		issuer = c.Issuer.String()
	}
	return &Info{Issuer: issuer, Subject: c.Subject.CommonName, Names: namesOf(c), NotBefore: c.NotBefore, NotAfter: c.NotAfter}, nil
}

// Covers reports whether the certificate at the start of fullchain is valid for every one of the names.
func Covers(fullchain []byte, names []string) bool {
	all, err := certificates(fullchain)
	if err != nil || len(all) == 0 {
		return false
	}
	for _, n := range names {
		if all[0].VerifyHostname(n) != nil {
			return false
		}
	}
	return true
}

// NewCSR makes a private key and a request for a certificate that covers names, the first being the common name.
// algo is "ecdsa" (P-256, the default) or "rsa" (2048 bits, for authorities that still ask for it).
func NewCSR(algo string, names []string) (keyPEM, csrPEM []byte, err error) {
	if len(names) == 0 {
		return nil, nil, errors.New("no name for the request")
	}
	var key crypto.Signer
	switch algo {
	case "", "ecdsa":
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "rsa":
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	default:
		return nil, nil, fmt.Errorf("key type %q is not one of: ecdsa, rsa", algo)
	}
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: names[0]}, DNSNames: names}, key)
	if err != nil {
		return nil, nil, err
	}
	kd, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}
