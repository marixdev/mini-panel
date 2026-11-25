package dbadmin

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// Remote access: the user of a database can also be reached from addresses outside this machine. Each address is a user of its own on
// the server ('blog'@'203.0.113.5'), made with the same password as 'blog'@'localhost' (its hash is copied, so the site's settings
// stay as they are), and with the same rights on the same database only.

// AnyHost is the host of a user that may connect from anywhere.
const AnyHost = "%"

// MaxHosts is the most addresses one database may be reached from.
const MaxHosts = 20

// NormalizeHost checks one address as a person types it and returns it the way the server and the firewall want it: an IPv4 or IPv6
// address, an IPv4 network such as 203.0.113.0/24, or "%" for anywhere. ok is false for anything else, a host name included: a name
// would make the server look it up on every connection.
func NormalizeHost(s string) (host string, ok bool) {
	s = strings.TrimSpace(s)
	if s == AnyHost {
		return AnyHost, true
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip.String(), true
	}
	ip, n, err := net.ParseCIDR(s)
	if err != nil || ip.To4() == nil {
		return "", false // the server takes IPv6 ranges in no form that MariaDB and MySQL share
	}
	if ones, _ := n.Mask.Size(); ones == 32 {
		return ip.String(), true
	} else if ones < 8 {
		return "", false
	}
	return n.String(), true
}

// ParseHosts splits a list written with commas, spaces or new lines, normalizes each address and drops repeats.
func ParseHosts(s string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' || r == '\t' || r == ';' }) {
		h, ok := NormalizeHost(f)
		if !ok {
			return nil, fmt.Errorf("%q is not an IP address, an IPv4 network such as 203.0.113.0/24, or %%", f)
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	if len(out) > MaxHosts {
		return nil, fmt.Errorf("at most %d addresses", MaxHosts)
	}
	return out, nil
}

// userHost is how a host is written in the user's name on the server. An IPv4 network is written with its mask, which every version of
// MariaDB and MySQL reads (the /24 form only newer MySQL does).
func userHost(h string) string {
	if _, n, err := net.ParseCIDR(h); err == nil {
		return n.IP.String() + "/" + net.IP(n.Mask).String()
	}
	return h
}

var hostCharsRe = regexp.MustCompile(`^[0-9A-Fa-f.:/%]{1,60}$`)

// Hosts returns the hosts of the users of a database other than localhost, as NormalizeHost writes them.
func (a Admin) Hosts(name string) ([]string, error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("invalid name %q", name)
	}
	out, err := a.R.Run(fmt.Sprintf("SELECT host FROM mysql.user WHERE user = '%s' AND host <> 'localhost' ORDER BY host;\n", name))
	if err != nil {
		return nil, err
	}
	var hosts []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if ip, mask, ok := strings.Cut(line, "/"); ok { // 203.0.113.0/255.255.255.0
			if m := net.ParseIP(mask).To4(); m != nil {
				ones, _ := net.IPMask(m).Size()
				line = fmt.Sprintf("%s/%d", ip, ones)
			}
		}
		hosts = append(hosts, line)
	}
	return hosts, nil
}

// createLikeSQL turns what SHOW CREATE USER printed for 'name'@'localhost' into the statement that makes 'name'@'host' with the same
// password. Only the host is replaced, and only in the user's name at the start, so the hash is copied as it is.
func createLikeSQL(shown, name, host string) (string, error) {
	shown = strings.TrimSpace(shown)
	for _, q := range []string{"`", "'"} {
		head := "CREATE USER " + q + name + q + "@" + q + "localhost" + q
		if strings.HasPrefix(shown, head) {
			return "CREATE USER '" + name + "'@'" + userHost(host) + "'" + shown[len(head):] + ";\n", nil
		}
	}
	return "", errors.New("the server described the user in a way mpn does not know")
}

// SetHosts makes the users of a database for exactly these hosts: it adds the ones that are missing, with the password of the local
// user, and drops the ones that are no longer wanted. The local user is never touched.
func (a Admin) SetHosts(name string, hosts []string, mysql bool) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid name %q", name)
	}
	for _, h := range hosts {
		if n, ok := NormalizeHost(h); !ok || n != h || !hostCharsRe.MatchString(h) {
			return fmt.Errorf("invalid host %q", h)
		}
	}
	have, err := a.Hosts(name)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, h := range hosts {
		want[h] = true
	}
	var b strings.Builder
	for _, h := range have {
		if !want[h] && hostCharsRe.MatchString(h) {
			fmt.Fprintf(&b, "DROP USER IF EXISTS '%s'@'%s';\n", name, userHost(h))
		}
	}
	missing := []string{}
	haveSet := map[string]bool{}
	for _, h := range have {
		haveSet[h] = true
	}
	for _, h := range hosts {
		if !haveSet[h] {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		show := fmt.Sprintf("SHOW CREATE USER '%s'@'localhost';\n", name)
		if mysql { // MySQL prints the hash of caching_sha2_password as raw bytes unless asked for hex
			show = "SET SESSION print_identified_with_as_hex = ON;\n" + show
		}
		shown, err := a.R.Run(show)
		if err != nil {
			return err
		}
		for _, h := range missing {
			stmt, err := createLikeSQL(shown, name, h)
			if err != nil {
				return err
			}
			b.WriteString(stmt)
			fmt.Fprintf(&b, "GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%s';\n", escapeGrantDB(name), name, userHost(h))
		}
	}
	if b.Len() == 0 {
		return nil
	}
	_, err = a.R.Run(b.String())
	return err
}
