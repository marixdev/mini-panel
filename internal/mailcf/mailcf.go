// Package mailcf turns the DNS records that a mail domain needs (internal/mail) into records for the API of Cloudflare (internal/cfdns).
// It makes no request and reads no file.
package mailcf

import (
	"fmt"
	"strconv"
	"strings"

	"mpn/internal/cfdns"
	"mpn/internal/mail"
)

func clean(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

// Wanted returns the records to add, in the order they are added: the address of the mail host (only when hostIP is given), then MX,
// SPF, DKIM and DMARC. hostIP is given only when the caller found that the mail host has no address yet and the machine has a public
// one: a mail host that is already set up is not touched. The address record is never proxied, because mail does not pass through the
// proxy of Cloudflare. TTL 1 is "automatic" for Cloudflare.
func Wanted(host string, recs []mail.Record, hostIP string) ([]cfdns.Record, error) {
	var out []cfdns.Record
	if hostIP != "" {
		if err := mail.ValidDomain(clean(host)); err != nil {
			return nil, fmt.Errorf("mail host: %w", err)
		}
		typ := "A"
		if strings.Contains(hostIP, ":") {
			typ = "AAAA"
		}
		out = append(out, cfdns.Record{Type: typ, Name: clean(host), Content: hostIP, TTL: 1, Proxied: false})
	}
	for _, r := range recs {
		switch r.Type {
		case "MX":
			pri, target, ok := strings.Cut(strings.TrimSpace(r.Value), " ")
			n, err := strconv.Atoi(pri)
			if !ok || err != nil || n < 0 || n > 65535 || clean(target) == "" {
				return nil, fmt.Errorf("the MX record %q is not \"priority host\"", r.Value)
			}
			out = append(out, cfdns.Record{Type: "MX", Name: clean(r.Name), Content: clean(target), Priority: &n, TTL: 1})
		case "TXT":
			if strings.TrimSpace(r.Value) == "" {
				return nil, fmt.Errorf("the TXT record %s has no value", r.Name)
			}
			out = append(out, cfdns.Record{Type: "TXT", Name: clean(r.Name), Content: r.Value, TTL: 1})
		default:
			return nil, fmt.Errorf("the panel does not add a %s record to Cloudflare", r.Type)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("there is nothing to add")
	}
	return out, nil
}
