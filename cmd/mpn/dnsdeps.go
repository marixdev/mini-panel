package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
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

// probeClient follows redirects as Let's Encrypt does: up to 10, to http or https on the same name. A name behind the proxy of
// Cloudflare with "Always use HTTPS" answers the first request with a redirect to https; the authority follows it, and so must
// this check, or such a name is refused although a certificate could be had. A redirect to another name is not followed: the file
// has to come from this name. The certificate of the https answer is not checked, as the authority does not check it either (the
// site has none yet, or a self-signed one behind the proxy).
func probeClient(host string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, Proxy: nil},
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			// A relative redirect stays on the address of the first request, which is the IP; the name is kept in Host.
			same := strings.EqualFold(r.URL.Hostname(), host) || r.URL.Hostname() == via[0].URL.Hostname()
			if len(via) >= 10 || !same || (r.URL.Scheme != "http" && r.URL.Scheme != "https") {
				return http.ErrUseLastResponse
			}
			r.Host = host
			return nil
		},
	}
}

// probeOurFile puts a file with a random name where the certificate authority would look, then asks the web server at ip for it
// with host as the Host header. Only this machine can answer with the content, so a machine behind NAT, whose public address is on
// no network card, still proves that the name leads to it, and so does a name behind a proxy such as Cloudflare's, whose address
// is the proxy's: the proxy passes the request on to this machine.
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
	resp, err := probeClient(host).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	return resp.StatusCode == 200 && strings.TrimSpace(string(body)) == token
}
