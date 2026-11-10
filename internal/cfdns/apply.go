package cfdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Status is what happened to one wanted record.
type Status string

const (
	Added    Status = "added"
	Exists   Status = "exists"   // the same record was already there; nothing was done
	Conflict Status = "conflict" // a different record stands where this one would go; nothing was done
	Failed   Status = "failed"
)

// Result is the outcome for one wanted record, in the order they were asked for.
type Result struct {
	Record Record
	Status Status
	Note   string
}

var quotedRe = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

// normalizeTXT turns the value Cloudflare returns (often in quotes, long values in several quoted pieces) and the value we send into one
// comparable text: the pieces joined, quotes removed, runs of white space made one space. Case is kept, because a DKIM key is case-sensitive.
func normalizeTXT(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if m := quotedRe.FindAllStringSubmatch(s, -1); len(m) > 0 {
			var b strings.Builder
			for _, p := range m {
				b.WriteString(strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(p[1]))
			}
			s = b.String()
		}
	}
	return strings.Join(strings.Fields(s), " ")
}

func normalize(w Record) Record {
	r := w
	r.Type = strings.ToUpper(strings.TrimSpace(w.Type))
	r.Name = cleanName(w.Name)
	switch r.Type {
	case "TXT":
		r.Content = normalizeTXT(w.Content)
	case "MX", "CNAME":
		r.Content = cleanName(w.Content)
	case "A", "AAAA":
		r.Content = strings.TrimSpace(w.Content)
		if ip := net.ParseIP(r.Content); ip != nil {
			r.Content = ip.String()
		}
		r.Proxied = false // mail does not go through the proxy of Cloudflare, so the address of a mail host is always "DNS only"
	}
	if r.TTL == 0 {
		r.TTL = 1
	}
	return r
}

func validate(r Record) error {
	if r.Name == "" || strings.ContainsAny(r.Name, " \t\r\n/\\\"'") {
		return errors.New("the name of the record is not valid")
	}
	if r.Content == "" {
		return errors.New("the record has no value")
	}
	switch r.Type {
	case "A":
		if ip := net.ParseIP(r.Content); ip == nil || ip.To4() == nil {
			return errors.New("an A record needs an IPv4 address")
		}
	case "AAAA":
		if ip := net.ParseIP(r.Content); ip == nil || ip.To4() != nil {
			return errors.New("an AAAA record needs an IPv6 address")
		}
	case "MX":
		if r.Priority == nil || *r.Priority < 0 || *r.Priority > 65535 {
			return errors.New("an MX record needs a priority from 0 to 65535")
		}
	case "TXT":
		if len(r.Content) > 2048 {
			return errors.New("a TXT value is limited to 2048 characters")
		}
	case "CNAME":
	default:
		return fmt.Errorf("a %s record is not one that is added here", r.Type)
	}
	return nil
}

// family says what a TXT record is for, so that two of the same kind at one name are a conflict and an unrelated one (a site verification
// string) is not. The name settles DKIM (a selector) and DMARC (_dmarc); SPF is told by its first word.
func familyOf(name, content string) string {
	n := cleanName(name)
	switch {
	case strings.Contains(n, "._domainkey."):
		return "dkim"
	case strings.HasPrefix(n, "_dmarc."):
		return "dmarc"
	case strings.HasPrefix(strings.ToLower(normalizeTXT(content)), "v=spf1"):
		return "spf"
	}
	return "other"
}

func samePriority(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// same says whether two records are the one: names and host names without regard to case or a final dot, a TXT value by its text.
func same(a, b Record) bool {
	if !strings.EqualFold(a.Type, b.Type) || cleanName(a.Name) != cleanName(b.Name) {
		return false
	}
	switch strings.ToUpper(a.Type) {
	case "TXT":
		return normalizeTXT(a.Content) == normalizeTXT(b.Content)
	case "MX":
		return cleanName(a.Content) == cleanName(b.Content) && samePriority(a.Priority, b.Priority)
	case "CNAME":
		return cleanName(a.Content) == cleanName(b.Content)
	}
	return normalize(a).Content == normalize(b).Content
}

func describe(e Record) string {
	if e.Priority != nil {
		return fmt.Sprintf("%s %d %s", e.Type, *e.Priority, e.Content)
	}
	return e.Type + " " + e.Content
}

// decide compares a wanted record with the records at its name. It returns "" when the record should be added.
func decide(r Record, at []Record) (Status, string) {
	var sameType []Record
	for _, e := range at {
		if same(r, e) {
			return Exists, "already there"
		}
		if strings.EqualFold(e.Type, r.Type) {
			sameType = append(sameType, e)
		}
	}
	for _, e := range at {
		switch {
		case r.Type != "CNAME" && strings.EqualFold(e.Type, "CNAME"):
			return Conflict, "a CNAME stands at this name and blocks other records (" + e.Content + "); not changed"
		case r.Type == "CNAME" && !strings.EqualFold(e.Type, "CNAME"):
			return Conflict, "there are other records at this name, which a CNAME cannot share (" + describe(e) + "); not changed"
		}
	}
	if r.Type == "TXT" {
		fam := familyOf(r.Name, r.Content)
		if fam == "other" {
			return "", ""
		}
		for _, e := range sameType {
			if familyOf(e.Name, e.Content) == fam {
				return Conflict, "there is already a different " + strings.ToUpper(fam) + " record (" + describe(e) + "); not changed"
			}
		}
		return "", ""
	}
	if len(sameType) > 0 { // A, AAAA, MX, CNAME: a different one where the mail goes is for the person to decide
		var have []string
		for _, e := range sameType {
			have = append(have, describe(e))
		}
		return Conflict, "there is already " + strings.Join(have, ", ") + " at this name; not changed"
	}
	return "", ""
}

// Apply adds the wanted records to the zone, one after another, and says what became of each. It never changes or removes a record that is
// there. After the token is refused or the rate limit is reached it stops calling the API and marks the rest as failed.
func (c Client) Apply(ctx context.Context, zoneID string, want []Record) []Result {
	out := make([]Result, 0, len(want))
	cache := map[string][]Record{} // the records at each name, kept up to date as records are added
	var stop error
	for _, w := range want {
		r := normalize(w)
		res := Result{Record: r}
		finish := func(s Status, note string) {
			res.Status, res.Note = s, c.scrub(note)
			out = append(out, res)
		}
		if err := validate(r); err != nil {
			finish(Failed, err.Error())
			continue
		}
		if stop != nil {
			finish(Failed, "not tried: "+stop.Error())
			continue
		}
		at, ok := cache[r.Name]
		if !ok {
			var err error
			if at, err = c.ListRecords(ctx, zoneID, "", r.Name); err != nil {
				stop = fatal(err, ctx)
				finish(Failed, err.Error())
				continue
			}
			cache[r.Name] = at
		}
		if s, note := decide(r, at); s != "" {
			finish(s, note)
			continue
		}
		made, err := c.CreateRecord(ctx, zoneID, r)
		if err != nil {
			var ae *APIError
			if errors.As(err, &ae) && (ae.has(81057) || ae.has(81058)) { // the identical record appeared meanwhile
				finish(Exists, "already there")
				continue
			}
			stop = fatal(err, ctx)
			finish(Failed, err.Error())
			continue
		}
		cache[r.Name] = append(cache[r.Name], made)
		finish(Added, "")
	}
	return out
}

// fatal returns err when trying the next record could not work either.
func fatal(err error, ctx context.Context) error {
	if errors.Is(err, ErrBadToken) || errors.Is(err, ErrNoPermission) || errors.Is(err, ErrRateLimited) || ctx.Err() != nil {
		return err
	}
	return nil
}
