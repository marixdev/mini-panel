// Package panelssl holds what is fixed about the certificate of the panel itself: where it lives, which names the panel
// answers to, and the nginx block that lets Let's Encrypt reach it. Like the firewall package it makes text and checks
// values; writing files and asking the authority is the work of the caller.
package panelssl

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"mpn/internal/security"
	"mpn/internal/site"
)

const (
	// Dir is read by the nginx blocks of both the panel and its database browser, so one certificate here serves both ports.
	Dir = "/etc/mpn/certs/_panel"
	// HostsFile lists the names the panel answers to besides the ones it was started with. The panel reads it while it runs.
	HostsFile = "/etc/mpn/panel-hosts"
	// ACMEConf answers the challenge of Let's Encrypt on port 80 for the name of the panel, and sends other visitors on to it.
	ACMEConf = "/etc/nginx/conf.d/04-mpn-panel-acme.conf"

	ModeSelfSigned  = "selfsigned"
	ModeLetsEncrypt = "letsencrypt"
	ModeCustom      = "custom"

	// Settings that remember what the person chose.
	SettingMode   = "panel_ssl"
	SettingDomain = "panel_domain"
)

// ValidDomain accepts a lower-case host name with at least one dot. An address, "localhost" and a name that is only a label
// cannot get a certificate from a public authority, and an address is always accepted by the panel anyway.
func ValidDomain(d string) error {
	switch {
	case d == "" || len(d) > 253:
		return fmt.Errorf("enter the name the panel is reached by, such as panel.example.com")
	case net.ParseIP(strings.Trim(d, "[]")) != nil:
		return fmt.Errorf("%q is an IP address; a certificate needs a name", d)
	case !strings.Contains(d, ".") || !site.ValidHostname(d):
		return fmt.Errorf("%q is not a lower-case host name such as panel.example.com", d)
	}
	return nil
}

// ACMEConfig is the nginx block for the name of the panel. port is where visitors who come to port 80 are sent.
func ACMEConfig(domain string, port int) (string, error) {
	if err := ValidDomain(domain); err != nil {
		return "", err
	}
	if port < 1024 || port > 65535 {
		return "", fmt.Errorf("panel port %d is out of range", port)
	}
	return "# Written by mpn: lets Let's Encrypt check the name of the panel and sends visitors of port 80 to the panel.\n" +
		"server {\n    listen 80;\n    listen [::]:80;\n    server_name " + domain + ";\n    " + security.OffInclude + "\n" +
		"    location /.well-known/acme-challenge/ { root /var/lib/mpn/acme; }\n" +
		"    location / { return 301 https://$host:" + strconv.Itoa(port) + "$request_uri; }\n}\n", nil
}

// HostsText is the content of HostsFile for one name.
func HostsText(domain string) string { return domain + "\n" }

// ParseHosts reads HostsFile: one name per line, blank lines and lines that are not a valid name are ignored, so a damaged
// file can only fail to add a name, never add something odd.
func ParseHosts(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if d := strings.ToLower(strings.TrimSpace(line)); ValidDomain(d) == nil {
			out = append(out, d)
		}
	}
	return out
}
