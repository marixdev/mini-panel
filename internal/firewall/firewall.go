// Package firewall makes the rules of the server's firewall: a table of its own in nftables, "inet mpn", that filters what
// comes in and touches nothing else. The ports of SSH and of the panel are always open and are not rules a person can delete,
// because a mistake here would cut the person off from the machine they are managing.
package firewall

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	Allow = "allow"
	Deny  = "deny"

	MaxRules   = 200
	MaxComment = 80
)

// Rule is one line a person added.
type Rule struct {
	ID      int64  `json:"id"`
	Action  string `json:"action"`  // Allow or Deny
	Proto   string `json:"proto"`   // "tcp", "udp" or "both"
	Port    string `json:"port"`    // "" for every port, "443" or "8000-8100"
	Source  string `json:"source"`  // "" for every address, or an address or a network such as 203.0.113.0/24
	Comment string `json:"comment"` // only for the person; it is not written to the firewall
}

// Locked are the ports that stay open whatever the rules say.
type Locked struct {
	SSH   []int `json:"ssh"`
	Panel []int `json:"panel"`
}

// ValidPort checks "" (every port), a port or a range of ports.
func ValidPort(p string) error {
	if p == "" {
		return nil
	}
	lo, hi := p, p
	if i := strings.IndexByte(p, '-'); i >= 0 {
		lo, hi = p[:i], p[i+1:]
	}
	a, err1 := strconv.Atoi(lo)
	b, err2 := strconv.Atoi(hi)
	if err1 != nil || err2 != nil || strconv.Itoa(a) != lo || strconv.Itoa(b) != hi || a < 1 || b > 65535 || a > b {
		return fmt.Errorf("the port %q must be a number from 1 to 65535, or a range such as 8000-8100", p)
	}
	return nil
}

// ParseSource reads an address or a network and returns it in its plain form, with its family (4 or 6).
func ParseSource(s string) (string, int, error) {
	if s == "" {
		return "", 0, nil
	}
	if ip := net.ParseIP(s); ip != nil && !strings.ContainsAny(s, "%/") {
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), 4, nil
		}
		return ip.String(), 6, nil
	}
	if _, n, err := net.ParseCIDR(s); err == nil && !strings.ContainsAny(s, "%") {
		if n.IP.To4() != nil {
			return n.String(), 4, nil
		}
		return n.String(), 6, nil
	}
	return "", 0, fmt.Errorf("the source %q must be an IP address or a network such as 203.0.113.0/24", s)
}

// ValidRule checks every part of a rule and returns the rule with the source in its plain form.
func ValidRule(r Rule) (Rule, error) {
	if r.Action != Allow && r.Action != Deny {
		return r, fmt.Errorf("the action %q must be allow or deny", r.Action)
	}
	if r.Proto != "tcp" && r.Proto != "udp" && r.Proto != "both" {
		return r, fmt.Errorf("the protocol %q must be tcp, udp or both", r.Proto)
	}
	if err := ValidPort(r.Port); err != nil {
		return r, err
	}
	src, _, err := ParseSource(r.Source)
	if err != nil {
		return r, err
	}
	r.Source = src
	switch {
	case !utf8.ValidString(r.Comment) || strings.ContainsAny(r.Comment, "\r\n\x00"):
		return r, fmt.Errorf("the comment must be one line of text")
	case utf8.RuneCountInString(r.Comment) > MaxComment:
		return r, fmt.Errorf("the comment is longer than %d characters", MaxComment)
	}
	r.Comment = strings.TrimSpace(r.Comment)
	if r.Action == Allow && r.Port == "" && r.Source == "" {
		return r, fmt.Errorf("a rule that allows every port from every address would make the firewall pointless; give a port or a source")
	}
	if r.Action == Deny && r.Port == "" && r.Source == "" {
		return r, fmt.Errorf("a rule that blocks every port from every address would cut off everyone; give a port or a source")
	}
	return r, nil
}

// portsInRange reports whether the port, or any port of the range, is one of the list. An empty port is every port.
func portsInRange(port string, list []int) bool {
	if port == "" {
		return len(list) > 0
	}
	lo, hi := port, port
	if i := strings.IndexByte(port, '-'); i >= 0 {
		lo, hi = port[:i], port[i+1:]
	}
	a, _ := strconv.Atoi(lo)
	b, _ := strconv.Atoi(hi)
	for _, p := range list {
		if p >= a && p <= b {
			return true
		}
	}
	return false
}

// CheckLockout refuses a rule that would cut the person off. A rule that blocks a port for everyone, when that port is the one
// of SSH or of the panel, would do it; so would one that blocks the address the person is working from.
func CheckLockout(r Rule, l Locked, client string) error {
	if r.Action != Deny {
		return nil
	}
	way := append(append([]int{}, l.SSH...), l.Panel...)
	if r.Source == "" && portsInRange(r.Port, way) {
		return fmt.Errorf("this rule would block the port of SSH or of the panel for everyone, and you could not log in again")
	}
	if client != "" && Covers(r.Source, client) && portsInRange(r.Port, way) {
		return fmt.Errorf("this rule would block your own address %s from SSH or the panel", client)
	}
	return nil
}

// Covers reports whether source (an address or a network, or "" for everyone) includes the address ip.
func Covers(source, ip string) bool {
	if source == "" {
		return true
	}
	a := net.ParseIP(ip)
	if a == nil {
		return false
	}
	if p := net.ParseIP(source); p != nil {
		return p.Equal(a)
	}
	if _, n, err := net.ParseCIDR(source); err == nil {
		return n.Contains(a)
	}
	return false
}

func ports(list []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, p := range list {
		if p >= 1 && p <= 65535 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}

func portSet(list []int) string {
	s := make([]string, len(list))
	for i, p := range list {
		s[i] = strconv.Itoa(p)
	}
	return "{ " + strings.Join(s, ", ") + " }"
}

func ruleLines(r Rule) []string {
	verdict := map[string]string{Allow: "accept", Deny: "drop"}[r.Action]
	_, fam, _ := ParseSource(r.Source)
	src := ""
	if r.Source != "" {
		src = map[int]string{4: "ip saddr ", 6: "ip6 saddr "}[fam] + r.Source + " "
	}
	protos := []string{r.Proto}
	if r.Proto == "both" {
		protos = []string{"tcp", "udp"}
	}
	if r.Port == "" && r.Proto == "both" {
		return []string{src + verdict}
	}
	var out []string
	for _, p := range protos {
		if r.Port == "" {
			out = append(out, fmt.Sprintf("%smeta l4proto %s %s", src, p, verdict))
		} else {
			out = append(out, fmt.Sprintf("%s%s dport %s %s", src, p, r.Port, verdict))
		}
	}
	return out
}

// Render makes the script that nft loads. It replaces the table "inet mpn" in one step, so there is no moment when the
// machine has half of the rules. Blocked sources come first, then the ports that never close, then the rules that allow.
// Whatever nothing allows is dropped.
func Render(l Locked, rules []Rule) string {
	var b strings.Builder
	b.WriteString("# Made by mpn. Changes made here are overwritten; use the panel.\n")
	b.WriteString("table inet mpn\ndelete table inet mpn\n\ntable inet mpn {\n  chain input {\n    type filter hook input priority filter; policy drop;\n")
	b.WriteString("    iif lo accept\n    ct state established,related accept\n    ct state invalid drop\n")
	for _, r := range rules {
		if r.Action == Deny {
			for _, line := range ruleLines(r) {
				b.WriteString("    " + line + "\n")
			}
		}
	}
	b.WriteString("    ip protocol icmp accept\n    ip6 nexthdr icmpv6 accept\n")
	b.WriteString("    udp sport 67 udp dport 68 accept\n    udp dport 546 accept\n")
	if p := ports(l.SSH); len(p) > 0 {
		b.WriteString("    tcp dport " + portSet(p) + " accept\n")
	}
	if p := ports(l.Panel); len(p) > 0 {
		b.WriteString("    tcp dport " + portSet(p) + " accept\n")
	}
	for _, r := range rules {
		if r.Action == Allow {
			for _, line := range ruleLines(r) {
				b.WriteString("    " + line + "\n")
			}
		}
	}
	b.WriteString("  }\n}\n")
	return b.String()
}

// Defaults are the rules a new firewall starts with: the web.
func Defaults() []Rule {
	return []Rule{
		{Action: Allow, Proto: "tcp", Port: "80", Comment: "HTTP"},
		{Action: Allow, Proto: "tcp", Port: "443", Comment: "HTTPS"},
	}
}

// Unload is the script that takes the firewall away.
const Unload = "table inet mpn\ndelete table inet mpn\n"
