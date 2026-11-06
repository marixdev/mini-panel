// Package dnscheck tells whether the names of a site lead to this machine, before a certificate is asked for. Let's Encrypt
// allows only a few failed validations an hour for a name, so a request that is bound to fail should not be made.
package dnscheck

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// Marker starts the message of every failure, so that a caller (the web panel) can recognise it in the output of mpn.
const Marker = "dns-check-failed"

// Deps are the three things the check needs from the world. They are separate so that a test can supply its own.
type Deps struct {
	// Lookup returns every A and AAAA address of a name.
	Lookup func(ctx context.Context, host string) ([]net.IP, error)
	// Local reports whether an address belongs to this machine.
	Local func(ip net.IP) bool
	// Probe asks the web server at ip for a file that only this machine can have put there, using host as the Host header.
	// It is the same request the certificate authority makes, and it is what settles the case of a machine behind NAT, whose
	// public address is on no network card.
	Probe func(ctx context.Context, ip net.IP, host string) bool
}

// Failure says which name does not lead here and why.
type Failure struct {
	Name     string
	NoRecord bool     // the name has no address at all
	Others   []net.IP // addresses of the name that are not this machine's
	Err      error    // the lookup failed
}

func (f *Failure) Error() string {
	switch {
	case f.Err != nil:
		return fmt.Sprintf("%s: %s: the address could not be looked up: %v", Marker, f.Name, f.Err)
	case f.NoRecord:
		return fmt.Sprintf("%s: %s: the name has no A or AAAA record", Marker, f.Name)
	}
	ips := make([]string, len(f.Others))
	for i, ip := range f.Others {
		ips[i] = ip.String()
	}
	return fmt.Sprintf("%s: %s: it points to %s, which is not this server", Marker, f.Name, strings.Join(ips, ", "))
}

// Check returns nil when every name leads to this machine. A name with an address that is not this machine's fails, even when
// another address of the same name is right: the certificate authority may pick either one.
func Check(ctx context.Context, d Deps, names []string) *Failure {
	for _, n := range names {
		ips, err := d.Lookup(ctx, n)
		if err != nil {
			return &Failure{Name: n, Err: err}
		}
		if len(ips) == 0 {
			return &Failure{Name: n, NoRecord: true}
		}
		var others []net.IP
		for _, ip := range ips {
			if d.Local(ip) || (d.Probe != nil && d.Probe(ctx, ip, n)) {
				continue
			}
			others = append(others, ip)
		}
		if len(others) > 0 {
			return &Failure{Name: n, Others: others}
		}
	}
	return nil
}
