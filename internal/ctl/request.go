// Package ctl is the only way the web panel, which runs without privileges, can ask for work. A Request names one
// operation from a fixed list and carries typed values. Argv checks every value and turns the request into the argument
// list of an mpn command; nothing the web side sends is ever run through a shell or placed on a command line unchecked.
package ctl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"mpn/internal/backup"
	"mpn/internal/dbadmin"
	"mpn/internal/site"
	"mpn/internal/vhost"
)

type Request struct {
	Op        string   `json:"op"`
	Site      string   `json:"site,omitempty"`
	Name      string   `json:"name,omitempty"` // a database name
	Kind      string   `json:"kind,omitempty"`
	User      string   `json:"user,omitempty"`
	Hosts     []string `json:"hosts,omitempty"`
	PHP       string   `json:"php,omitempty"`
	Node      string   `json:"node,omitempty"`
	Python    string   `json:"python,omitempty"` // a python site's interpreter, such as 3.12
	Entry     string   `json:"entry,omitempty"`
	Upstream  string   `json:"upstream,omitempty"`
	Purge     bool     `json:"purge,omitempty"`
	Backup    string   `json:"backup,omitempty"` // a backup name, or "latest"
	Files     bool     `json:"files,omitempty"`
	All       bool     `json:"all,omitempty"`
	Databases []string `json:"databases,omitempty"`
	Keep      int      `json:"keep,omitempty"`
	Enable    bool     `json:"enable,omitempty"`   // site.access: allow (true) or forbid (false) logging in as the site's user
	Password  string   `json:"password,omitempty"` // site.access with Enable: the password; reaches mpn on stdin only
	// site.set: the settings of a site. All of those that belong to its kind are sent every time, so what a form shows is what is stored.
	RootSub string            `json:"root_sub,omitempty"` // web root below htdocs; empty is htdocs itself
	Port    int               `json:"port,omitempty"`     // port of a node app
	Mem     int               `json:"mem,omitempty"`      // memory of a node app in MB, 0 for the default
	PHPIni  map[string]string `json:"php_ini,omitempty"`  // PHP settings that differ from the defaults
	// site.domain: the domain of a site and its www choice. Both are sent every time.
	Domain string `json:"domain,omitempty"`
	WWW    string `json:"www,omitempty"` // "", "bare" or "www"
	// SSL. site.sslcustom sends the three boxes of the form; they reach mpn on standard input only.
	Key            string `json:"key,omitempty"`   // private key; empty uses the one made by site.csr
	Cert           string `json:"cert,omitempty"`  // certificate, or the whole bundle
	Chain          string `json:"chain,omitempty"` // intermediate certificates
	AllowUntrusted bool   `json:"allow_untrusted,omitempty"`
	Algo           string `json:"algo,omitempty"`     // site.csr: ecdsa or rsa
	Redirect       bool   `json:"redirect,omitempty"` // site.sslredirect: send http:// visitors to SSL
	// site.vhostset: the nginx configuration written by hand. It reaches mpn on standard input only.
	Vhost string `json:"vhost,omitempty"`
}

// Stdin returns what the request sends to mpn on standard input: the password of site.access, the certificate and key of
// site.sslcustom, and nothing otherwise. It never appears in the argument list, in the log, or in the answer.
func (r Request) Stdin() string {
	switch {
	case r.Op == "site.access" && r.Enable:
		return r.Password + "\n"
	case r.Op == "site.vhostset":
		return r.Vhost
	case r.Op == "site.sslcustom":
		b, _ := json.Marshal(struct {
			Key            string `json:"key"`
			Cert           string `json:"cert"`
			Chain          string `json:"chain"`
			AllowUntrusted bool   `json:"allow_untrusted"`
		}{r.Key, r.Cert, r.Chain, r.AllowUntrusted})
		return string(b)
	}
	return ""
}

// maxCertText bounds each box of the certificate form. A chain of four certificates is under 8 KB.
const maxCertText = 16 << 10

// writes lists the operations that change the machine. Everything else only reads.
var writes = map[string]bool{
	"site.add": true, "site.apply": true, "site.remove": true, "site.access": true, "site.panelkey": true, "site.set": true, "site.domain": true,
	"site.ssl": true, "site.sslcustom": true, "site.csr": true, "site.ssloff": true, "site.sslredirect": true, // where the certificate comes from
	"site.vhostset": true, "site.vhostreset": true, // the nginx configuration of a site
	"db.add": true, "db.passwd": true, "db.remove": true,
	"db.pma": true, "db.pmadrop": true, "db.pmaclean": true, // the temporary users of the database browser
	"backup.create": true, "backup.restore": true, "backup.prune": true,
}

var reads = map[string]bool{"site.list": true, "site.info": true, "site.sslinfo": true, "site.vhost": true, "db.list": true, "backup.list": true}

// Known reports whether op is an operation at all.
func Known(op string) bool { return writes[op] || reads[op] }

// IsWrite reports whether op changes the machine; the helper runs those one at a time.
func IsWrite(op string) bool { return writes[op] }

func optionalSite(s string) error {
	if s != "" && !site.ValidName(s) {
		return fmt.Errorf("invalid site name %q", s)
	}
	return nil
}

func requireSite(s string) error {
	if !site.ValidName(s) {
		return fmt.Errorf("invalid site name %q", s)
	}
	return nil
}

func requireDB(name string) error {
	if !dbadmin.ValidName(name) {
		return fmt.Errorf("invalid database name %q", name)
	}
	return nil
}

// ownedDB accepts a database name only when it is clearly the site's own.
func ownedDB(name, siteName string) error {
	if err := requireDB(name); err != nil {
		return err
	}
	if !dbadmin.OwnedBy(name, siteName) {
		return fmt.Errorf("database %q is not named for site %q", name, siteName)
	}
	return nil
}

// Argv checks the request and returns the arguments for the mpn binary, or an error naming the first problem.
func (r Request) Argv() ([]string, error) {
	if !Known(r.Op) {
		return nil, fmt.Errorf("unknown operation %q", r.Op)
	}
	switch r.Op {
	case "site.list":
		return []string{"site", "list", "-json"}, nil
	case "site.add":
		return r.addSite()
	case "site.info":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"site", "info", "-name", r.Site}, nil
	case "site.set":
		return r.setSite()
	case "site.domain":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		if !site.ValidDomain(r.Domain) {
			return nil, fmt.Errorf("domain %q must be a lower-case host name such as example.com", r.Domain)
		}
		if !site.ValidWWW(r.WWW) {
			return nil, fmt.Errorf("www choice %q must be empty, bare or www", r.WWW)
		}
		return []string{"site", "domain", "-name", r.Site, "-domain", r.Domain, "-www", r.WWW, "-apply"}, nil
	case "site.ssl", "site.ssloff", "site.sslinfo", "site.sslcustom", "site.csr", "site.sslredirect":
		return r.sslArgv()
	case "site.vhost", "site.vhostset", "site.vhostreset":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		switch r.Op {
		case "site.vhost":
			return []string{"site", "vhost", "-name", r.Site}, nil
		case "site.vhostreset":
			return []string{"site", "vhost", "-name", r.Site, "-reset", "-apply"}, nil
		}
		if len(r.Vhost) > vhost.MaxCustom {
			return nil, fmt.Errorf("the configuration is longer than %d KB", vhost.MaxCustom>>10)
		}
		if strings.TrimSpace(r.Vhost) == "" || strings.ContainsRune(r.Vhost, 0) {
			return nil, fmt.Errorf("the configuration is empty or has a character that cannot be part of a text file")
		}
		return []string{"site", "vhost", "-name", r.Site, "-stdin", "-apply"}, nil
	case "site.apply":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"site", "apply", "-name", r.Site, "-apply"}, nil
	case "site.remove":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		a := []string{"site", "remove", "-name", r.Site}
		if r.Purge {
			a = append(a, "-purge")
		}
		return append(a, "-apply"), nil
	case "site.panelkey":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"site", "access", "-name", r.Site, "-panelkey", "-apply"}, nil
	case "site.access":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		if !r.Enable {
			return []string{"site", "access", "-name", r.Site, "-disable", "-apply"}, nil
		}
		if !dbadmin.ValidPassword(r.Password) {
			return nil, fmt.Errorf("invalid password: 12 to 64 printable characters without space, quotes, backslash or backtick")
		}
		return []string{"site", "access", "-name", r.Site, "-enable", "-stdin", "-apply"}, nil
	case "db.list":
		if err := optionalSite(r.Site); err != nil {
			return nil, err
		}
		a := []string{"db", "list", "-json"}
		if r.Site != "" {
			a = append(a, "-site", r.Site)
		}
		return a, nil
	case "db.add":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		a := []string{"db", "add", "-site", r.Site}
		if r.Name != "" {
			if err := ownedDB(r.Name, r.Site); err != nil {
				return nil, err
			}
			a = append(a, "-name", r.Name)
		}
		return append(a, "-apply"), nil
	case "db.passwd", "db.remove":
		if err := requireDB(r.Name); err != nil {
			return nil, err
		}
		return []string{"db", r.Op[3:], "-name", r.Name, "-apply"}, nil
	case "db.pma":
		if err := requireDB(r.Name); err != nil {
			return nil, err
		}
		return []string{"db", "pma", "-name", r.Name, "-apply"}, nil
	case "db.pmadrop":
		if !dbadmin.ValidTempName(r.User) {
			return nil, fmt.Errorf("invalid temporary user %q", r.User)
		}
		return []string{"db", "pmadrop", "-user", r.User, "-apply"}, nil
	case "db.pmaclean":
		return []string{"db", "pmaclean", "-apply"}, nil
	case "backup.list":
		if err := optionalSite(r.Site); err != nil {
			return nil, err
		}
		a := []string{"backup", "list", "-json"}
		if r.Site != "" {
			a = append(a, "-site", r.Site)
		}
		return a, nil
	case "backup.create":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"backup", "create", "-site", r.Site, "-apply"}, nil
	case "backup.prune":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		if r.Keep < 1 || r.Keep > 365 {
			return nil, fmt.Errorf("keep must be between 1 and 365")
		}
		return []string{"backup", "prune", "-site", r.Site, "-keep", strconv.Itoa(r.Keep), "-apply"}, nil
	}
	return r.restore() // the only operation left
}

func (r Request) restore() ([]string, error) {
	if err := requireSite(r.Site); err != nil {
		return nil, err
	}
	if r.Backup != "latest" && !backup.ValidStamp(r.Backup) {
		return nil, fmt.Errorf("invalid backup name %q", r.Backup)
	}
	a := []string{"backup", "restore", "-site", r.Site, "-backup", r.Backup}
	switch {
	case r.All:
		a = append(a, "-all")
	case !r.Files && len(r.Databases) == 0:
		return nil, fmt.Errorf("choose what to restore: the files, a database, or everything")
	default:
		if r.Files {
			a = append(a, "-files")
		}
		for _, d := range r.Databases {
			if err := ownedDB(d, r.Site); err != nil {
				return nil, err
			}
			a = append(a, "-database", d)
		}
	}
	return append(a, "-apply"), nil
}

func (r Request) addSite() ([]string, error) {
	user := r.User
	if user == "" {
		user = r.Site
	}
	s := site.Site{Name: r.Site, Kind: site.Kind(r.Kind), User: user, Root: "/home/" + user + "/htdocs",
		Hostnames: r.Hosts, PHPVersion: r.PHP, NodeVersion: r.Node, Entry: r.Entry, Upstream: r.Upstream}
	a := []string{"site", "add", "-name", s.Name, "-kind", string(s.Kind), "-user", user}
	switch s.Kind {
	case site.PHP:
		if s.PHPVersion == "" {
			s.PHPVersion = "8.4"
		}
		a = append(a, "-php", s.PHPVersion)
	case site.Node:
		if s.NodeVersion == "" {
			s.NodeVersion = "22"
		}
		if s.Entry == "" {
			s.Entry = "server.js"
		}
		s.NodePort = 20000 // only for the check; the real port is picked by mpn
		a = append(a, "-node", s.NodeVersion, "-entry", s.Entry)
	case site.Python:
		s.PyVersion = r.Python
		if s.PyVersion == "" {
			s.PyVersion = "3.12" // only for the check; mpn uses the system's python3 when none is given
		}
		if s.Entry == "" {
			s.Entry = "app.py"
		}
		s.NodePort = 20000
		a = append(a, "-entry", s.Entry)
		if r.Python != "" {
			a = append(a, "-python", r.Python)
		}
	case site.Proxy:
		a = append(a, "-upstream", s.Upstream)
	}
	if len(s.Hostnames) > 20 {
		return nil, fmt.Errorf("at most 20 hostnames")
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	for _, h := range s.Hostnames {
		a = append(a, "-host", h)
	}
	return a, nil
}

// sslArgv turns the SSL operations into the arguments of the matching mpn command. The text of a certificate and key is not
// an argument; it goes on standard input (see Stdin).
func (r Request) sslArgv() ([]string, error) {
	if err := requireSite(r.Site); err != nil {
		return nil, err
	}
	switch r.Op {
	case "site.ssl":
		return []string{"site", "ssl", "-name", r.Site, "-apply"}, nil
	case "site.ssloff":
		return []string{"site", "ssl-off", "-name", r.Site, "-apply"}, nil
	case "site.sslinfo":
		return []string{"site", "ssl-info", "-name", r.Site}, nil
	case "site.sslredirect":
		return []string{"site", "ssl-redirect", "-name", r.Site, "-redirect=" + strconv.FormatBool(r.Redirect), "-apply"}, nil
	case "site.csr":
		if r.Algo != "" && r.Algo != "ecdsa" && r.Algo != "rsa" {
			return nil, fmt.Errorf("key type %q is not one of: ecdsa, rsa", r.Algo)
		}
		algo := r.Algo
		if algo == "" {
			algo = "ecdsa"
		}
		return []string{"site", "ssl-csr", "-name", r.Site, "-algo", algo, "-apply"}, nil
	}
	// site.sslcustom
	for name, text := range map[string]string{"key": r.Key, "certificate": r.Cert, "chain": r.Chain} {
		if len(text) > maxCertText {
			return nil, fmt.Errorf("the %s is longer than %d KB", name, maxCertText>>10)
		}
		if strings.ContainsRune(text, 0) {
			return nil, fmt.Errorf("the %s has a character that cannot be part of a PEM text", name)
		}
	}
	if strings.TrimSpace(r.Cert) == "" {
		return nil, fmt.Errorf("the certificate is empty")
	}
	return []string{"site", "ssl-custom", "-name", r.Site, "-apply"}, nil
}

// setSite checks the settings of "site.set" against the kind of site and turns them into the arguments of "mpn site set".
// The kind comes from the request and is checked again by mpn against the stored site, so a wrong kind cannot change
// anything that does not belong to the site.
func (r Request) setSite() ([]string, error) {
	if err := requireSite(r.Site); err != nil {
		return nil, err
	}
	a := []string{"site", "set", "-name", r.Site}
	switch site.Kind(r.Kind) {
	case site.Static, site.PHP, site.Node, site.Python:
		if !site.ValidSubdir(r.RootSub) {
			return nil, fmt.Errorf("web root %q: use folder names of letters, digits, '.', '_' and '-', separated by '/'", r.RootSub)
		}
		a = append(a, "-root-sub", r.RootSub)
	case site.Proxy:
	default:
		return nil, fmt.Errorf("unknown kind %q", r.Kind)
	}
	switch site.Kind(r.Kind) {
	case site.PHP:
		if !site.ValidPHPVersion(r.PHP) {
			return nil, fmt.Errorf("php version %q is not offered", r.PHP)
		}
		if err := site.ValidatePHPIni(r.PHPIni); err != nil {
			return nil, err
		}
		a = append(a, "-php", r.PHP, "-ini-reset")
		keys := make([]string, 0, len(r.PHPIni))
		for k := range r.PHPIni {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			a = append(a, "-ini", k+"="+r.PHPIni[k])
		}
	case site.Node:
		switch {
		case !site.ValidNodeVersion(r.Node):
			return nil, fmt.Errorf("node version %q is not offered", r.Node)
		case !site.ValidEntry(r.Entry):
			return nil, fmt.Errorf("entry %q must be a relative path to the start script", r.Entry)
		case !site.ValidAppPort(r.Port):
			return nil, fmt.Errorf("port %d must be between 1024 and 65535 and not one that the server itself uses", r.Port)
		case !site.ValidMemMB(r.Mem):
			return nil, fmt.Errorf("memory %d MB must be 0 (the default) or between 64 and 8192", r.Mem)
		}
		a = append(a, "-node", r.Node, "-entry", r.Entry, "-port", strconv.Itoa(r.Port), "-mem", strconv.Itoa(r.Mem))
	case site.Python:
		switch {
		case !site.ValidPythonVersion(r.Python):
			return nil, fmt.Errorf("python version %q must look like 3.12", r.Python)
		case !site.ValidPythonEntry(r.Entry):
			return nil, fmt.Errorf("entry %q must be a relative path to the start script, or -m and a module name", r.Entry)
		case !site.ValidAppPort(r.Port):
			return nil, fmt.Errorf("port %d must be between 1024 and 65535 and not one that the server itself uses", r.Port)
		case !site.ValidMemMB(r.Mem):
			return nil, fmt.Errorf("memory %d MB must be 0 (the default) or between 64 and 8192", r.Mem)
		}
		a = append(a, "-python", r.Python, "-entry", r.Entry, "-port", strconv.Itoa(r.Port), "-mem", strconv.Itoa(r.Mem))
	case site.Proxy:
		if !site.ValidUpstream(r.Upstream) {
			return nil, fmt.Errorf("upstream %q must be a plain http or https URL", r.Upstream)
		}
		a = append(a, "-upstream", r.Upstream)
	}
	return append(a, "-apply"), nil
}
