// Package cfdns finds out whether a domain is served by Cloudflare and adds DNS records to its zone through the Cloudflare API with a token the
// person pastes. It uses the standard library only, keeps the token in memory for the length of a call, and never writes it anywhere: not in
// an error, not in a result, not when a Client is printed.
//
// It only adds. A record that is already there is left alone, and one that differs is reported as a conflict, never changed or deleted: what
// a domain's mail goes to is the person's decision.
package cfdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

// The kinds of failure a caller tells apart with errors.Is.
var (
	ErrBadToken     = errors.New("cloudflare: the API token is not valid")
	ErrNoPermission = errors.New("cloudflare: the API token may not do this")
	ErrNoZone       = errors.New("cloudflare: the domain is not a zone of this account")
	ErrRateLimited  = errors.New("cloudflare: too many requests, try again in a minute")
	ErrNetwork      = errors.New("cloudflare: the API could not be reached")
)

// NSResolver is what Detect needs of a resolver; *net.Resolver satisfies it.
type NSResolver interface {
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
}

// Detection says whether the name servers of a domain are Cloudflare's. Zone is the name whose name servers were looked at (the domain
// itself or the nearest parent that has any), set only when Cloudflare is true.
type Detection struct {
	Cloudflare bool
	Zone       string
}

func cleanName(s string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
}

// Detect looks for the name servers of domain, then of each parent with at least two labels, and stops at the first name that has any: a
// subdomain with name servers of its own is not served by the parent's. A name that does not exist is skipped; any other DNS failure (a
// timeout, a refused query) is returned, because "I could not tell" must not read as "not Cloudflare".
func Detect(ctx context.Context, r NSResolver, domain string) (Detection, error) {
	name := cleanName(domain)
	if name == "" {
		return Detection{}, errors.New("cfdns: no domain")
	}
	labels := strings.Split(name, ".")
	for i := 0; len(labels)-i >= 2; i++ {
		cand := strings.Join(labels[i:], ".")
		ns, err := r.LookupNS(ctx, cand)
		if err != nil {
			var de *net.DNSError
			if errors.As(err, &de) && de.IsNotFound {
				continue
			}
			return Detection{}, fmt.Errorf("cfdns: looking up the name servers of %s: %w", cand, err)
		}
		if len(ns) == 0 {
			continue
		}
		for _, n := range ns {
			if n != nil && strings.HasSuffix(cleanName(n.Host), ".ns.cloudflare.com") {
				return Detection{Cloudflare: true, Zone: cand}, nil
			}
		}
		return Detection{}, nil
	}
	return Detection{}, nil
}
