package mailcf

import (
	"context"
	"strings"

	"mpn/internal/cfdns"
	"mpn/internal/mail"
)

// Report is what Add did.
type Report struct {
	Zone    string
	Results []cfdns.Result
}

// inZone says name is the zone or a name below it.
func inZone(name, zone string) bool {
	name, zone = clean(name), clean(zone)
	return name == zone || strings.HasSuffix(name, "."+zone)
}

// hostHasAddress says the mail host already has an address record (or a CNAME that stands in its place) in the zone, in which case the
// panel leaves it alone: it may point somewhere on purpose.
func hostHasAddress(ctx context.Context, c cfdns.Client, zoneID, host string) (bool, error) {
	for _, typ := range []string{"A", "AAAA", "CNAME"} {
		recs, err := c.ListRecords(ctx, zoneID, typ, clean(host))
		if err != nil {
			return false, err
		}
		if len(recs) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// Add puts the DNS records a mail domain needs into its zone at Cloudflare. It finds the zone from the domain, adds the address of the
// mail host only when hostIP is given, the host is below the same zone and has no address yet, and then adds MX, SPF, DKIM and DMARC.
// A record that is already there is left; a different one in the same place is reported and not touched. Nothing is deleted or changed.
func Add(ctx context.Context, c cfdns.Client, domain, host string, recs []mail.Record, hostIP string) (Report, error) {
	zone, err := c.FindZone(ctx, domain)
	if err != nil {
		return Report{}, err
	}
	if hostIP != "" {
		if !inZone(host, zone.Name) {
			hostIP = "" // the host is in another zone, which this token may not reach
		} else if has, err := hostHasAddress(ctx, c, zone.ID, host); err != nil {
			return Report{}, err
		} else if has {
			hostIP = ""
		}
	}
	want, err := Wanted(host, recs, hostIP)
	if err != nil {
		return Report{}, err
	}
	return Report{Zone: zone.Name, Results: c.Apply(ctx, zone.ID, want)}, nil
}
