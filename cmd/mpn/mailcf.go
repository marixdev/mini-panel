package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"mpn/internal/cfdns"
	"mpn/internal/mail"
	"mpn/internal/mailcf"
	"mpn/internal/store"
)

// The DNS records of a mail domain can be put into Cloudflare when the domain's name servers are Cloudflare's. The person brings an API
// token made for that zone; it is read from standard input, used for this one request and not kept anywhere.

// publicIPv4 is the first address of a network card that is neither private nor in the range carriers use for shared addresses, or "".
// A machine behind NAT has none, and then the address of the mail host is left for the person to add.
func publicIPv4() string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		n, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := n.IP.To4()
		if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || (ip[0] == 100 && ip[1]&0xc0 == 64) {
			continue
		}
		return ip.String()
	}
	return ""
}

type cfDetectInfo struct {
	Cloudflare bool   `json:"cloudflare"`
	Zone       string `json:"zone,omitempty"`
}

func cfDetect(domain string) (cfDetectInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	d, err := cfdns.Detect(ctx, net.DefaultResolver, domain)
	if err != nil {
		return cfDetectInfo{}, err
	}
	return cfDetectInfo{Cloudflare: d.Cloudflare, Zone: d.Zone}, nil
}

// cfMarker turns an error of the Cloudflare client into a line that starts with a fixed word, which the web side recognises and says in
// Vietnamese. None of these messages holds the token.
func cfMarker(err error) error {
	switch {
	case errors.Is(err, cfdns.ErrBadToken):
		return fmt.Errorf("cf-badtoken: %w", err)
	case errors.Is(err, cfdns.ErrNoPermission):
		return fmt.Errorf("cf-noperm: %w", err)
	case errors.Is(err, cfdns.ErrNoZone):
		return fmt.Errorf("cf-nozone: %w", err)
	case errors.Is(err, cfdns.ErrRateLimited):
		return fmt.Errorf("cf-ratelimit: %w", err)
	case errors.Is(err, cfdns.ErrNetwork):
		return fmt.Errorf("cf-network: %w", err)
	}
	return err
}

type cfAddRow struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

type cfAddReport struct {
	Zone    string     `json:"zone"`
	HostIP  string     `json:"host_ip,omitempty"`
	Results []cfAddRow `json:"results"`
}

func readToken() (string, error) {
	line, err := bufio.NewReader(io.LimitReader(os.Stdin, 512)).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	tok := strings.TrimSpace(line)
	if !cfdns.ValidToken(tok) {
		return "", errors.New("cf-badtoken: the API token must be 20 to 128 letters, digits, \"_\" or \"-\"")
	}
	return tok, nil
}

// cfAdd adds the records of one mail domain to its zone at Cloudflare, or says what it would add when apply is false.
func cfAdd(st *store.Store, domain string, apply, asJSON bool) {
	var list []mail.Record
	found := false
	for _, d := range currentMail(st).Domains {
		if d.Name == domain {
			found, list = true, d.Records
		}
	}
	if !found {
		fail(fmt.Errorf("the domain %s is not set up for mail", domain))
	}
	host, _ := st.MailHost()
	if host == "" {
		fail(errors.New("mail-off: turn mail on first; the records name the mail host"))
	}
	ip := publicIPv4()
	if !apply {
		want, err := mailcf.Wanted(host, list, ip)
		fail(err)
		for _, w := range want {
			fmt.Printf("%s\t%s\n", w.Type, w.Name)
		}
		fmt.Println("# checked only. Add -apply, with the API token on standard input, to add them to Cloudflare.")
		return
	}
	tok, err := readToken()
	fail(err)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	rep, err := mailcf.Add(ctx, cfdns.Client{Token: tok}, domain, host, list, ip)
	fail(cfMarker(err))
	out := cfAddReport{Zone: rep.Zone, HostIP: ip, Results: []cfAddRow{}}
	for _, r := range rep.Results {
		out.Results = append(out.Results, cfAddRow{Type: r.Record.Type, Name: r.Record.Name, Status: string(r.Status), Note: r.Note})
	}
	if asJSON {
		printJSON(out)
		return
	}
	for _, r := range out.Results {
		fmt.Printf("%-9s %-4s %s %s\n", r.Status, r.Type, r.Name, r.Note)
	}
}
