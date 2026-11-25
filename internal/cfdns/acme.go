package cfdns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// TXTLookup asks the name servers of a zone for the TXT records of a name; *net.Resolver's LookupTXT has this shape.
type TXTLookup func(ctx context.Context, name string) ([]string, error)

// ACME puts the TXT record of the dns-01 challenge of Let's Encrypt into a zone at Cloudflare and takes it away again. It works for a
// name whatever is in front of the web server, the proxy of Cloudflare included, and for a name that does not lead here at all.
type ACME struct {
	Client Client
	Lookup TXTLookup     // nil: the system resolver
	Wait   time.Duration // the longest wait for the record to be seen; 0 is 2 minutes
	Every  time.Duration // between two looks; 0 is 5 seconds
}

// Present adds "_acme-challenge.<name>" TXT value, waits until a resolver sees it, and returns a function that deletes that record (only
// that one: a record of the same name added by somebody else stays).
func (a ACME) Present(ctx context.Context, name, value string) (func(), error) {
	name = cleanName(name)
	zone, err := a.Client.FindZone(ctx, name)
	if err != nil {
		return nil, err
	}
	rec := "_acme-challenge." + name
	made, err := a.Client.CreateRecord(ctx, zone.ID, Record{Type: "TXT", Name: rec, Content: value, TTL: 60})
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		if made.ID == "" {
			return
		}
		c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = a.Client.DeleteRecord(c, zone.ID, made.ID)
	}
	if err := a.waitFor(ctx, rec, value); err != nil {
		cleanup()
		return nil, err
	}
	return cleanup, nil
}

func (a ACME) waitFor(ctx context.Context, rec, value string) error {
	look := a.Lookup
	if look == nil {
		look = net.DefaultResolver.LookupTXT
	}
	wait, every := a.Wait, a.Every
	if wait == 0 {
		wait = 2 * time.Minute
	}
	if every == 0 {
		every = 5 * time.Second
	}
	deadline := time.Now().Add(wait)
	for {
		if txts, err := look(ctx, rec); err == nil {
			for _, t := range txts {
				if strings.Trim(t, `"`) == value {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the TXT record %s was added at Cloudflare but was not seen within %s", rec, wait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(every):
		}
	}
}
