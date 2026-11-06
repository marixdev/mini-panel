package acme

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math"
	"time"
)

// DaysLeft returns the whole days until the first certificate in chainPEM expires. It is negative once expired.
func DaysLeft(chainPEM []byte, now time.Time) (int, error) {
	block, _ := pem.Decode(chainPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return 0, errors.New("no certificate found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return 0, err
	}
	return int(math.Floor(cert.NotAfter.Sub(now).Hours() / 24)), nil
}
