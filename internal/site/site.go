// Package site defines the hosted-site model and validates it before anything is rendered or stored.
package site

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

type Kind string

const (
	Static Kind = "static"
	PHP    Kind = "php"
	Node   Kind = "node"
	Proxy  Kind = "proxy"
	Python Kind = "python"
)

// pythonVersionRe matches the minor version of a Python 3 interpreter such as "3.12". Python sites use the interpreters the
// system packages provide (/usr/bin/python3.N), so the list of versions is read from the machine, not fixed here.
var pythonVersionRe = regexp.MustCompile(`^3\.(?:[0-9]|[1-3][0-9])$`)

// ValidPythonVersion reports whether v looks like the minor version of a Python 3 interpreter.
func ValidPythonVersion(v string) bool { return pythonVersionRe.MatchString(v) }

// pythonEntryRe matches what a Python app is started with: a script relative to the web root, or "-m" and a module name.
var pythonEntryRe = regexp.MustCompile(`^(?:[A-Za-z0-9_][A-Za-z0-9._/-]*|-m [A-Za-z_][A-Za-z0-9_.]*)$`)

// ValidPythonEntry reports whether e is a safe start script or "-m module" for a Python app.
func ValidPythonEntry(e string) bool {
	return pythonEntryRe.MatchString(e) && !strings.Contains(e, "..")
}

// VenvDir is the virtual environment of a Python site. The site's own packages go here, never into the system's Python.
func VenvDir(user string) string { return "/home/" + user + "/venv" }

// PHPVersions are the versions offered. 8.6 is left out until it is confirmed as a stable release.
var PHPVersions = []string{"7.0", "7.1", "7.2", "7.3", "7.4", "8.0", "8.1", "8.2", "8.3", "8.4", "8.5"}

// NodeVersions are the Node.js major versions offered. Older ones are past end of life; the UI should warn.
var NodeVersions = []string{"18", "20", "22", "24"}

var entryRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/-]*$`)

type Site struct {
	Name        string
	Kind        Kind
	User        string   // system account that owns the files and runs the app
	Root        string   // absolute document root
	Hostnames   []string // the first one is the primary name
	PHPVersion  string   // php only
	NodePort    int      // node and python: the port the app listens on (named for the first kind that used it)
	NodeVersion string   // node only, a major version such as "22"
	PyVersion   string   // python only, a minor version such as "3.12"
	Entry       string   // node and python: start script relative to Root, such as "server.js"; python also accepts "-m module"
	Upstream    string   // proxy only, an http or https URL
	TLS         bool     // HTTPS is on; set once a certificate is installed
	// Settings a person may change later (see settings.go).
	PHPIni map[string]string // php only: the settings that differ from the defaults; validated against a fixed list
	MemMB  int               // node and python: memory the app may use in MB; 0 is the default
	// The domain choices (see domain.go).
	WWW       string // WWWOff, WWWBare or WWWWith
	AllowHTTP bool   // with SSL on, still serve over plain HTTP instead of sending visitors to SSL
	SSLMode   string // SSLNone, SSLLetsEncrypt or SSLCustom; TLS is true exactly when this is not SSLNone
	// CustomVhost is the whole nginx configuration of the site when a person wrote it by hand on the vHost tab. It replaces the
	// one mpn makes, so settings that mpn writes into that one (domain, SSL, port, web root) no longer reach it.
	CustomVhost string
	// Speed turns on compression and long browser caching of static files (see vhost.go).
	Speed bool
	// App is the application mpn installed in a PHP site, one of Apps, or "" for a site whose code the person brought. It decides
	// which tabs and cards the site has (WordPress has a manager) and is set only once the installation has finished.
	App string
}

// Apps are the applications mpn can download and set up in a PHP site.
var Apps = []string{"wordpress", "laravel", "codeigniter", "cakephp", "yii"}

// ValidApp reports whether id names an application of Apps.
func ValidApp(id string) bool { return slices.Contains(Apps, id) }

// reservedUsers are accounts that exist on a stock Debian or Ubuntu server or that the software installed by mpn runs
// as. A site's account must be created for it, never one of these. Any other existing account is refused when the site
// is applied, because it does not carry the mark mpn puts on the accounts it creates.
var reservedUsers = map[string]bool{
	"root": true, "daemon": true, "bin": true, "sys": true, "sync": true, "games": true, "man": true, "lp": true,
	"mail": true, "news": true, "uucp": true, "proxy": true, "www-data": true, "backup": true, "list": true,
	"irc": true, "nobody": true, "nginx": true, "mysql": true, "mariadb": true, "redis": true, "memcache": true,
	"postfix": true, "sshd": true, "ubuntu": true, "debian": true, "admin": true, "administrator": true,
}

var (
	nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
	userRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	pathRe = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)
	hostRe = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

// IsReservedUser reports whether u is a system account name that a site may not use.
func IsReservedUser(u string) bool { return reservedUsers[u] || strings.HasPrefix(u, "systemd-") }

// ReservedUsers lists the reserved names in order, for forms that want to avoid them as the user types.
func ReservedUsers() []string { return slices.Sorted(maps.Keys(reservedUsers)) }

// ValidName reports whether n is an acceptable site name.
func ValidName(n string) bool { return nameRe.MatchString(n) }

// ValidHostname reports whether h is a lower-case ASCII host name that is safe to place in configuration.
func ValidHostname(h string) bool { return hostRe.MatchString(h) }

// SafePath reports whether p is an absolute path made only of characters that are harmless in nginx config.
func SafePath(p string) bool { return pathRe.MatchString(p) && !strings.Contains(p, "..") }

// Validate returns every problem found, joined, or nil when the site is safe to render.
func (s Site) Validate() error {
	var errs []error
	add := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }
	if !nameRe.MatchString(s.Name) {
		add("name %q: use a-z, 0-9 and '-', at most 31 characters", s.Name)
	}
	if !userRe.MatchString(s.User) {
		add("user %q is not a valid account name", s.User)
	} else if IsReservedUser(s.User) {
		add("user %q is a system account; every site gets its own new account, so pick another name", s.User)
	}
	if !SafePath(s.Root) {
		add("root %q must be an absolute path without '..' or special characters", s.Root)
	}
	if len(s.Hostnames) == 0 {
		add("at least one hostname is required")
	}
	for _, h := range s.Hostnames {
		if !hostRe.MatchString(h) {
			add("hostname %q is not a valid lower-case ASCII name", h)
		}
	}
	for _, p := range s.validDomainFields() {
		add("%s", p)
	}
	switch s.Kind {
	case Static:
	case PHP:
		if !slices.Contains(PHPVersions, s.PHPVersion) {
			add("php version %q is not offered", s.PHPVersion)
		}
		if err := ValidatePHPIni(s.PHPIni); err != nil {
			add("php settings: %v", err)
		}
		if s.App != "" && !ValidApp(s.App) {
			add("application %q is not one mpn installs", s.App)
		}
	case Node:
		if !ValidAppPort(s.NodePort) {
			add("node port %d must be between 1024 and 65535 and not one that the server itself uses", s.NodePort)
		}
		if !slices.Contains(NodeVersions, s.NodeVersion) {
			add("node version %q is not offered", s.NodeVersion)
		}
		if !entryRe.MatchString(s.Entry) || strings.Contains(s.Entry, "..") {
			add("entry %q must be a relative path to the start script", s.Entry)
		}
		if s.MemMB != 0 && (s.MemMB < 64 || s.MemMB > 8192) {
			add("memory %d MB must be 0 (the default) or between 64 and 8192", s.MemMB)
		}
	case Python:
		if !ValidAppPort(s.NodePort) {
			add("python port %d must be between 1024 and 65535 and not one that the server itself uses", s.NodePort)
		}
		if !ValidPythonVersion(s.PyVersion) {
			add("python version %q must look like 3.12", s.PyVersion)
		}
		if !ValidPythonEntry(s.Entry) {
			add("entry %q must be a relative path to the start script, or -m and a module name", s.Entry)
		}
		if !ValidMemMB(s.MemMB) {
			add("memory %d MB must be 0 (the default) or between 64 and 8192", s.MemMB)
		}
	case Proxy:
		if u, err := url.Parse(s.Upstream); err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
			u.Host == "" || strings.ContainsAny(s.Upstream, " \t\r\n;{}\"'\\$") {
			add("upstream %q must be a plain http or https URL", s.Upstream)
		}
	default:
		add("unknown kind %q", s.Kind)
	}
	return errors.Join(errs...)
}

// ProxyTarget returns the URL nginx forwards to, or "" for kinds that are not proxied.
func (s Site) ProxyTarget() string {
	switch s.Kind {
	case Node, Python:
		return fmt.Sprintf("http://127.0.0.1:%d", s.NodePort)
	case Proxy:
		return s.Upstream
	}
	return ""
}

// reservedPorts are ports the server's own services use. An app must not be given one: it would either fail to start or,
// worse, take the place of the service.
var reservedPorts = map[int]bool{
	22: true, 25: true, 80: true, 443: true, 465: true, 587: true, 993: true, 995: true, 3306: true, 33060: true, 5432: true,
	6379: true, 8780: true, 2610: true, 2611: true, 9000: true, 11211: true,
}

// ValidAppPort reports whether an app may listen on p.
func ValidAppPort(p int) bool { return p >= 1024 && p <= 65535 && !reservedPorts[p] }

func ValidPHPVersion(v string) bool  { return slices.Contains(PHPVersions, v) }
func ValidNodeVersion(v string) bool { return slices.Contains(NodeVersions, v) }

// ValidEntry reports whether e is a relative path to a start script.
func ValidEntry(e string) bool { return entryRe.MatchString(e) && !strings.Contains(e, "..") }

// ValidUpstream reports whether u is a plain http or https address that is safe to place in the web server's settings.
func ValidUpstream(u string) bool {
	p, err := url.Parse(u)
	return err == nil && (p.Scheme == "http" || p.Scheme == "https") && p.Host != "" && !strings.ContainsAny(u, " \t\r\n;{}\"'\\$")
}

// ValidMemMB reports whether m is 0 (the default) or a memory limit a node app may have.
func ValidMemMB(m int) bool { return m == 0 || (m >= 64 && m <= 8192) }
