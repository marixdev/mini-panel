package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mpn/internal/dnscheck"
)

const acmeWebRoot = "/var/lib/mpn/acme"

// realDNS is the check against the real world: the system's resolver, this machine's network cards, and a request to the web
// server at the address a name leads to.
func realDNS() dnscheck.Deps {
	return dnscheck.Deps{
		Lookup: func(ctx context.Context, host string) ([]net.IP, error) {
			addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				if dnsErr, ok := err.(*net.DNSError); ok && dnsErr.IsNotFound {
					return nil, nil // no record is an answer, not a failure of the lookup
				}
				return nil, err
			}
			ips := make([]net.IP, 0, len(addrs))
			for _, a := range addrs {
				ips = append(ips, a.IP)
			}
			return ips, nil
		},
		Local: func(ip net.IP) bool {
			addrs, _ := net.InterfaceAddrs()
			for _, a := range addrs {
				if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
					return true
				}
			}
			return false
		},
		Probe: probeOurFile,
	}
}

// probeOurFile puts a file with a random name where the certificate authority would look, then asks the web server at ip for it
// with host as the Host header. Only this machine can answer with the content, so a machine behind NAT, whose public address is on
// no network card, still proves that the name leads to it.
func probeOurFile(ctx context.Context, ip net.IP, host string) bool {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return false
	}
	token := hex.EncodeToString(buf)
	dir := filepath.Join(acmeWebRoot, ".well-known", "acme-challenge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	file := filepath.Join(dir, "mpn-check-"+token)
	if err := os.WriteFile(file, []byte(token), 0o644); err != nil {
		return false
	}
	defer os.Remove(file)

	host4 := ip.String()
	if ip.To4() == nil {
		host4 = "[" + ip.String() + "]"
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+host4+"/.well-known/acme-challenge/mpn-check-"+token, nil)
	if err != nil {
		return false
	}
	req.Host = host
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return resp.StatusCode == 200 && strings.TrimSpace(string(body)) == token
}
