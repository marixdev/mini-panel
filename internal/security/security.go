// Package security builds the configuration of fail2ban and of ModSecurity with the OWASP Core Rule Set. Like the firewall
// package it only makes text and checks values; installing packages and loading the result is the work of the caller.
package security

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// The three states of the web application firewall. Detect logs what the rules would block and blocks nothing.
const (
	ModeOff    = "off"
	ModeDetect = "detect"
	ModeBlock  = "block"
)

const (
	Dir      = "/etc/mpn/modsec"
	MainConf = Dir + "/main.conf"
	OffFile  = Dir + "/off.d/off.conf"
	// OffInclude goes into every server block that ModSecurity must leave alone (the panel, its database browser, a site whose
	// owner turned the firewall off). The glob matches nothing until the firewall is installed, and nginx accepts an include with
	// a glob that matches nothing, so the line is safe on a machine without the module, where "modsecurity off;" would not be.
	OffInclude = "include " + Dir + "/off.d/*.conf;"
	// NginxHook sits in conf.d, which nginx includes inside http {}: the rules are loaded once and every server block shares them.
	NginxHook = "/etc/nginx/conf.d/03-mpn-modsec.conf"

	JailFile         = "/etc/fail2ban/jail.d/mpn.conf"
	PanelFilterFile  = "/etc/fail2ban/filter.d/mpn-panel.conf"
	ModsecFilterFile = "/etc/fail2ban/filter.d/mpn-modsec.conf"
)

const (
	OffContent  = "modsecurity off;\n"
	HookContent = "modsecurity on;\nmodsecurity_rules_file " + MainConf + ";\n"

	// AuditLog is written by the nginx workers (user www-data), so the file is made for them. LogrotateFile keeps it small:
	// copytruncate, because ModSecurity keeps the file open and does not reopen it on a signal.
	AuditLog      = "/var/log/mpn/modsec_audit.log"
	LogrotateFile = "/etc/logrotate.d/mpn-modsec"
	LogrotateText = AuditLog + " {\n    daily\n    rotate 7\n    maxsize 20M\n    compress\n    delaycompress\n    missingok\n    notifempty\n    copytruncate\n}\n"
)

// ExemptScript puts OffInclude into a server block of the panel or of its database browser that was written before the firewall
// existed. It changes a file only when the include is missing, so it is safe to run again; nginx -t follows it in the plan.
var ExemptScript = `for f in /etc/nginx/conf.d/01-mpn-panel.conf /etc/nginx/conf.d/*mpn*pma*.conf; do
  [ -f "$f" ] || continue
  grep -q '` + Dir + `/off.d' "$f" || sed -i '/^ *server_name _;$/a\    ` + OffInclude + `' "$f"
done`

// Jails are the only jails mpn manages, and the only names the panel may ask about.
var Jails = []string{"sshd", "mpn-panel", "mpn-modsec"}

// ValidJail refuses any name that is not one of Jails.
func ValidJail(name string) error {
	for _, j := range Jails {
		if j == name {
			return nil
		}
	}
	return fmt.Errorf("unknown jail %q", name)
}

// ValidMode refuses anything but the three modes.
func ValidMode(m string) error {
	switch m {
	case ModeOff, ModeDetect, ModeBlock:
		return nil
	}
	return fmt.Errorf("mode %q is not off, detect or block", m)
}

// MainConfig is the rule file nginx loads. The packaged modsecurity.conf supplies the basics (body limits, the unicode map);
// the engine line after it decides whether a match only logs or also answers 403. Matches go to an audit log of their own
// (rotated by LogrotateContent): in detect mode ModSecurity reports them at the "warn" level, which the error log of a site
// does not record, so only a refused request reaches that log, as an error, where fail2ban reads it.
//
// exists tells whether a file is on the machine. CRS 3 and CRS 4 do not ship the same set of files (the exclusion files are
// named differently), and ModSecurity stops loading at an Include of a file that is missing, so only files that exist are named.
func MainConfig(mode string, exists func(path string) bool) (string, error) {
	engine := ""
	switch mode {
	case ModeDetect:
		engine = "DetectionOnly"
	case ModeBlock:
		engine = "On"
	default:
		return "", fmt.Errorf("mode %q has no rule file", mode)
	}
	if !exists(CRSRules) || !exists(CRSSetup) {
		return "", fmt.Errorf("the OWASP Core Rule Set is not installed (%s or %s is missing)", CRSRules, CRSSetup)
	}
	var b strings.Builder
	b.WriteString("# Written by mpn. Do not edit; change the mode in the panel.\n")
	if exists("/etc/nginx/modsecurity.conf") {
		b.WriteString("Include /etc/nginx/modsecurity.conf\n")
	}
	b.WriteString("SecRuleEngine " + engine + "\nSecAuditEngine RelevantOnly\nSecAuditLogParts ABHZ\nSecAuditLogType Serial\nSecAuditLog " + AuditLog + "\n")
	b.WriteString("Include " + CRSSetup + "\n")
	for _, f := range []string{"/etc/modsecurity/crs/REQUEST-900-EXCLUSION-RULES-BEFORE-CRS.conf"} {
		if exists(f) {
			b.WriteString("Include " + f + "\n")
		}
	}
	b.WriteString("Include " + CRSRules + "/*.conf\n")
	if exists("/etc/modsecurity/crs/RESPONSE-999-EXCLUSION-RULES-AFTER-CRS.conf") {
		b.WriteString("Include /etc/modsecurity/crs/RESPONSE-999-EXCLUSION-RULES-AFTER-CRS.conf\n")
	}
	return b.String(), nil
}

// Where the Debian and Ubuntu package modsecurity-crs puts the rules and the setup file.
const (
	CRSRules = "/usr/share/modsecurity-crs/rules"
	CRSSetup = "/etc/modsecurity/crs/crs-setup.conf"
)

// ValidAddr accepts one IP address, the form "unban" takes, and returns it in its short written form.
func ValidAddr(s string) (string, error) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil || a.Zone() != "" {
		return "", fmt.Errorf("%q is not an IP address", s)
	}
	return a.Unmap().String(), nil
}

// MaxIgnore bounds the list of addresses fail2ban never bans.
const MaxIgnore = 16

// ValidIgnore checks addresses and networks that fail2ban must never ban, drops repeats and keeps the order.
func ValidIgnore(list []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		var v string
		if p, err := netip.ParsePrefix(s); err == nil {
			v = p.Masked().String()
		} else if a, err := ValidAddr(s); err == nil {
			v = a
		} else {
			return nil, fmt.Errorf("%q is not an address or a network", s)
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if len(out) > MaxIgnore {
		return nil, fmt.Errorf("at most %d trusted addresses", MaxIgnore)
	}
	return out, nil
}

func portList(ports []int) (string, error) {
	if len(ports) == 0 {
		return "", fmt.Errorf("a jail needs at least one port")
	}
	sorted := append([]int(nil), ports...)
	sort.Ints(sorted)
	var out []string
	for i, p := range sorted {
		if p < 1 || p > 65535 {
			return "", fmt.Errorf("port %d is out of range", p)
		}
		if i > 0 && p == sorted[i-1] {
			continue
		}
		out = append(out, strconv.Itoa(p))
	}
	return strings.Join(out, ","), nil
}

// JailConfig is /etc/fail2ban/jail.d/mpn.conf. Bans go through nftables in the table fail2ban makes for itself, so they
// neither touch nor depend on the table "inet mpn" of the firewall page. The addresses in ignore (loopback and the one the
// person works from) are never banned.
func JailConfig(sshPorts, panelPorts []int, ignore []string) (string, error) {
	ssh, err := portList(sshPorts)
	if err != nil {
		return "", err
	}
	panel, err := portList(panelPorts)
	if err != nil {
		return "", err
	}
	trusted, err := ValidIgnore(ignore)
	if err != nil {
		return "", err
	}
	ign := append([]string{"127.0.0.0/8", "::1"}, trusted...)
	return "# Written by mpn. Do not edit; the panel rewrites this file.\n" +
		"[DEFAULT]\n" +
		"ignoreip = " + strings.Join(ign, " ") + "\n" +
		"banaction = nftables\n" +
		"banaction_allports = nftables[type=allports]\n" +
		"bantime = 1h\n" +
		"findtime = 10m\n\n" +
		"[sshd]\nenabled = true\nbackend = systemd\nport = " + ssh + "\nmaxretry = 5\n\n" +
		"[mpn-panel]\nenabled = true\nfilter = mpn-panel\nbackend = systemd\njournalmatch = _SYSTEMD_UNIT=mpn-web.service\nport = " + panel + "\nmaxretry = 5\n\n" +
		"[mpn-modsec]\nenabled = true\nfilter = mpn-modsec\nbackend = auto\nlogpath = /var/log/mpn/sites/*/error.log\nport = http,https\nmaxretry = 8\n", nil
}

// PanelFilter matches the line the panel writes to the journal for a wrong password (see server.go, login).
const PanelFilter = "[Definition]\nfailregex = login failed from <HOST>$\nignoreregex =\n"

// ModsecFilter matches the nginx error-log line of a request that ModSecurity refused. Warnings of the detect mode say
// "Warning", not "Access denied", so a site in detect mode never leads to a ban.
const ModsecFilter = "[Definition]\nfailregex = ^.*\\[error\\] \\d+#\\d+: \\*\\d+ .*ModSecurity: Access denied with code \\d+ .*, client: <HOST>,\nignoreregex =\n"
