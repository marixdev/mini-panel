// Package ctl is the only way the web panel, which runs without privileges, can ask for work. A Request names one
// operation from a fixed list and carries typed values. Argv checks every value and turns the request into the argument
// list of an mpn command; nothing the web side sends is ever run through a shell or placed on a command line unchecked.
package ctl

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mpn/internal/audit"
	"mpn/internal/backup"
	"mpn/internal/cloudprov"
	"mpn/internal/cloudsync"
	"mpn/internal/cron"
	"mpn/internal/dbadmin"
	"mpn/internal/firewall"
	"mpn/internal/notify"
	"mpn/internal/panelssl"
	"mpn/internal/security"
	"mpn/internal/services"
	"mpn/internal/site"
	"mpn/internal/sitelog"
	"mpn/internal/svcport"
	"mpn/internal/tune"
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
	Speed          bool   `json:"speed,omitempty"`    // site.speed: compress and cache static files
	// site.vhostset: the nginx configuration written by hand. It reaches mpn on standard input only.
	Vhost string `json:"vhost,omitempty"`
	// Scheduled jobs. site.cronadd sends the schedule and the command on standard input.
	Schedule string `json:"schedule,omitempty"`
	Command  string `json:"command,omitempty"`
	JobID    int64  `json:"job_id,omitempty"` // site.cronset and site.crondel
	On       bool   `json:"on,omitempty"`     // site.cronset: the job runs
	// The firewall. Client is the address of the person, so a rule that would block it is refused.
	FwAction  string `json:"fw_action,omitempty"`
	FwProto   string `json:"fw_proto,omitempty"`
	FwPort    string `json:"fw_port,omitempty"`
	FwSource  string `json:"fw_source,omitempty"`
	FwComment string `json:"fw_comment,omitempty"`
	RuleID    int64  `json:"rule_id,omitempty"`
	Client    string `json:"client,omitempty"`
	// ModSecurity with the OWASP Core Rule Set, and fail2ban.
	SecMode   string   `json:"sec_mode,omitempty"`   // security.waf: off, detect or block
	SecOn     bool     `json:"sec_on,omitempty"`     // security.f2b: fail2ban runs
	SecIgnore []string `json:"sec_ignore,omitempty"` // security.f2b: addresses and networks that are never banned
	Jail      string   `json:"jail,omitempty"`       // security.unban
	IP        string   `json:"ip,omitempty"`         // security.unban
	// The copy of the backups off the server. The client secret of Google and the settings of a storage reach mpn on standard input only.
	CloudClientID string              `json:"cloud_client_id,omitempty"` // cloud.connect
	CloudSecret   string              `json:"cloud_secret,omitempty"`    // cloud.connect
	CloudOn       bool                `json:"cloud_on,omitempty"`        // cloud.enable: the site is copied
	CloudStorage  *cloudprov.Settings `json:"cloud_storage,omitempty"`   // cloud.configure: S3, B2, SFTP, WebDAV or FTP
	// app.install: the application to download and set up in a PHP site, and for WordPress the details of the first administrator.
	// The password of the administrator is made by mpn and shown once; it is never sent here.
	App       string `json:"app,omitempty"`
	AppTitle  string `json:"app_title,omitempty"`
	AppAdmin  string `json:"app_admin,omitempty"`
	AppEmail  string `json:"app_email,omitempty"`
	AppLocale string `json:"app_locale,omitempty"`
	// The WordPress manager: wp.run does one thing to a plugin, a theme or WordPress itself; wp.search looks on wordpress.org.
	WPKind   string `json:"wp_kind,omitempty"`   // plugin, theme or core
	WPAction string `json:"wp_action,omitempty"` // install, activate, deactivate, update or delete
	WPSlug   string `json:"wp_slug,omitempty"`
	WPTerm   string `json:"wp_term,omitempty"`
	// The mail server. The password of a mailbox reaches mpn on standard input only; Purge (above) also deletes the stored mail.
	MailHost     string   `json:"mail_host,omitempty"`     // mail.enable: the name of this machine as a mail server
	MailDomain   string   `json:"mail_domain,omitempty"`   // the domain mail is received for
	MailAddress  string   `json:"mail_address,omitempty"`  // a mailbox or an alias
	MailTargets  []string `json:"mail_targets,omitempty"`  // mail.aliasset: where the mail of the alias goes
	MailQuota    int      `json:"mail_quota,omitempty"`    // size limit of a mailbox in MB, 0 for none
	MailPassword string   `json:"mail_password,omitempty"` // mail.boxadd and mail.boxpasswd; stdin only
	// site.log: which log of the site to read, how many of its last lines, and a plain text the lines must contain.
	LogKind string `json:"log_kind,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Query   string `json:"query,omitempty"`
	// services.control and services.install: a service of the fixed list in internal/services, and what to do to it.
	SvcKey    string `json:"svc_key,omitempty"`
	SvcAction string `json:"svc_action,omitempty"`
	// services.install of mariadb or mysql: the version to install, such as "11.8"; empty means the default of the engine.
	DBSeries string `json:"db_series,omitempty"`
	// panel.update: the release to install, such as "1.2.1"; empty means the latest.
	Version string `json:"version,omitempty"`
	// mail.cfadd: the API token of Cloudflare, made for the zone of MailDomain. It reaches mpn on standard input only and is not stored.
	// site.ssl: the same, for a certificate proved through DNS; it is kept for the renewals.
	CFToken string `json:"cf_token,omitempty"`
	// db.settings: the settings of the database server, as tune.CheckDBValues takes them. They reach mpn on standard input.
	DBValues map[string]string `json:"db_values,omitempty"`
	// The activity log. Actor is the address of the person who sent the request; the web side sets it on every request, and the root
	// helper writes it to the log with the operation. audit.note is an event only the web side sees (Event, one of audit.Events, with a
	// Note and whether it Failed). audit.list reads the log: Site, AuditIP, AuditKind, From and To choose, Limit and Offset page.
	// The Telegram alerts. Token (notify.hub) and Code (notify.agent, a pairing code) reach mpn on standard input only.
	Lang      string        `json:"lang,omitempty"`
	Token     string        `json:"token,omitempty"`
	Code      string        `json:"code,omitempty"`
	ChatID    int64         `json:"chat_id,omitempty"`
	NodeID    int64         `json:"node_id,omitempty"`
	HubURL    string        `json:"hub_url,omitempty"`
	Rules     *notify.Rules `json:"rules,omitempty"`
	Actor     string        `json:"actor,omitempty"`
	Event     string        `json:"event,omitempty"`
	Note      string        `json:"note,omitempty"`
	Failed    bool          `json:"failed,omitempty"`
	AuditIP   string        `json:"audit_ip,omitempty"`
	AuditKind string        `json:"audit_kind,omitempty"`
	From      string        `json:"from,omitempty"`
	To        string        `json:"to,omitempty"`
	Limit     int           `json:"limit,omitempty"`
	Offset    int           `json:"offset,omitempty"`
}

// Stdin returns what the request sends to mpn on standard input: the password of site.access, the certificate and key of
// site.sslcustom, and nothing otherwise. It never appears in the argument list, in the log, or in the answer.
func (r Request) Stdin() string {
	switch {
	case r.Op == "site.access" && r.Enable:
		return r.Password + "\n"
	case r.Op == "mail.boxadd", r.Op == "mail.boxpasswd":
		return r.MailPassword + "\n"
	case r.Op == "mail.cfadd", r.Op == "site.ssl" && r.CFToken != "":
		return r.CFToken + "\n"
	case r.Op == "db.settings":
		b, _ := json.Marshal(r.DBValues)
		return string(b)
	case r.Op == "notify.hub":
		return r.Token + "\n"
	case r.Op == "notify.agent":
		return r.Code + "\n"
	case r.Op == "site.vhostset":
		return r.Vhost
	case r.Op == "site.cronadd":
		b, _ := json.Marshal(struct {
			Schedule string `json:"schedule"`
			Command  string `json:"command"`
		}{r.Schedule, r.Command})
		return string(b)
	case r.Op == "cloud.connect":
		b, _ := json.Marshal(struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}{r.CloudClientID, r.CloudSecret})
		return string(b)
	case r.Op == "cloud.configure" && r.CloudStorage != nil:
		b, _ := json.Marshal(r.CloudStorage)
		return string(b)
	case r.Op == "site.sslcustom", r.Op == "panel.sslcustom":
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
	"site.vhostset": true, "site.vhostreset": true, "site.speed": true, // the nginx configuration of a site
	"site.cronadd": true, "site.cronset": true, "site.crondel": true, // the scheduled jobs of a site
	"firewall.enable": true, "firewall.disable": true, "firewall.add": true, "firewall.del": true,
	"port.set": true, "port.finish": true, "port.cancel": true, // the ports of SSH, the panel, phpMyAdmin and the web mail
	"security.waf": true, "security.f2b": true, "security.unban": true, // ModSecurity with the OWASP rules, and fail2ban
	"security.cloudflare": true,                                                // the visitor's address from CF-Connecting-IP on Cloudflare's connections
	"panel.ssl":           true, "panel.sslcustom": true, "panel.ssloff": true, // the certificate of the panel and of the database browser
	"cloud.connect": true, "cloud.finish": true, "cloud.disconnect": true, "cloud.enable": true, "cloud.sync": true, "cloud.fetch": true, // the copy of backups off the server
	"cloud.configure": true,                                                                                                               // connects S3, B2, SFTP, WebDAV or FTP and tests it
	"mail.enable":     true, "mail.disable": true, "mail.ssl": true, "mail.webmail": true, "mail.domainadd": true, "mail.domaindel": true, // the mail server and its domains
	"mail.cfadd":  true,                                                                     // adds the DNS records of a mail domain to Cloudflare; it changes nothing on this machine but acts on an account, so one at a time
	"mail.boxadd": true, "mail.boxdel": true, "mail.boxpasswd": true, "mail.boxquota": true, // mailboxes
	"mail.aliasset": true, "mail.aliasdel": true, // aliases
	"app.install": true, // download and set up WordPress or a framework in a PHP site
	"wp.run":      true, // an action of the WordPress manager
	"db.add":      true, "db.passwd": true, "db.remove": true, "db.remote": true, "db.settings": true,
	"db.pma": true, "db.pmadrop": true, "db.pmaclean": true, // the temporary users of the database browser
	"backup.create": true, "backup.restore": true, "backup.prune": true, "backup.keep": true,
	"services.control": true, "services.install": true, "services.remove": true, "site.webmail": true,
	"notify.test": true, "notify.off": true, "notify.hub": true, "notify.chat": true, "notify.rules": true, "notify.noderules": true, // the Telegram alerts
	"notify.pair": true, "notify.noderemove": true, "notify.agent": true,
	"panel.update": true, // replaces the program with a signed release (mpn update), in the background
}

var reads = map[string]bool{"port.status": true, "port.sshaddr": true, "notify.status": true, "audit.list": true, "audit.note": true, "mail.cfdetect": true, "site.list": true, "site.info": true, "site.log": true, "services.status": true, "site.sslinfo": true, "site.vhost": true, "site.cronlist": true, "firewall.status": true, "security.status": true, "mail.status": true, "mail.records": true, "panel.sslinfo": true, "cloud.status": true, "cloud.list": true, "wp.status": true, "wp.search": true, "db.list": true, "db.remoteinfo": true, "db.settingsinfo": true, "backup.list": true, "backup.keepinfo": true, "panel.updatecheck": true}

var versionRe = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}$`)

func validVersion(v string) bool { return versionRe.MatchString(v) }

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
	if err := audit.ValidActor(r.Actor); err != nil {
		return nil, err
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
	case "site.log":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		if err := sitelog.ValidKind(r.LogKind); err != nil {
			return nil, err
		}
		n, err := sitelog.ValidLines(r.Lines)
		if err != nil {
			return nil, err
		}
		if err := sitelog.ValidQuery(r.Query); err != nil {
			return nil, err
		}
		// "=" joins each value to its flag, so a search text that starts with "-" cannot be read as another flag.
		return []string{"site", "log", "-name", r.Site, "-kind=" + r.LogKind, "-lines=" + strconv.Itoa(n), "-query=" + r.Query}, nil
	case "services.status":
		return []string{"services", "status", "-json"}, nil
	case "panel.updatecheck":
		return []string{"update", "-check"}, nil
	case "panel.update":
		// Only a release number goes through; which release and whether it is genuine is decided by mpn update itself.
		if r.Version == "" {
			return []string{"update", "-apply", "-detach"}, nil
		}
		if !validVersion(r.Version) {
			return nil, fmt.Errorf("invalid version %q", r.Version)
		}
		return []string{"update", "-apply", "-detach", "-version=" + r.Version}, nil
	case "services.control":
		if err := services.ValidAction(r.SvcKey, r.SvcAction); err != nil {
			return nil, err
		}
		return []string{"services", "control", "-key=" + r.SvcKey, "-action=" + r.SvcAction, "-apply"}, nil
	case "services.remove":
		if err := services.ValidRemove(r.SvcKey); err != nil {
			return nil, err
		}
		return []string{"services", "remove", "-key=" + r.SvcKey, "-apply"}, nil
	case "services.install":
		if err := services.ValidInstall(r.SvcKey); err != nil {
			return nil, err
		}
		args := []string{"services", "install", "-key=" + r.SvcKey, "-apply"}
		if r.DBSeries != "" {
			if r.SvcKey != "mariadb" && r.SvcKey != "mysql" {
				return nil, fmt.Errorf("only a database server has a version to choose")
			}
			if err := services.ValidSeries(r.DBSeries); err != nil {
				return nil, err
			}
			args = append(args, "-series="+r.DBSeries)
		}
		return args, nil
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
	case "site.speed":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"site", "speed", "-name", r.Site, "-on=" + strconv.FormatBool(r.Speed), "-apply"}, nil
	case "site.webmail":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"site", "webmail", "-name", r.Site, "-on=" + strconv.FormatBool(r.Enable), "-apply"}, nil
	case "site.ssl", "site.ssloff", "site.sslinfo", "site.sslcustom", "site.csr", "site.sslredirect":
		return r.sslArgv()
	case "firewall.status":
		return []string{"firewall", "status", "-json"}, nil
	case "firewall.enable":
		return []string{"firewall", "enable", "-apply"}, nil
	case "firewall.disable":
		return []string{"firewall", "disable", "-apply"}, nil
	case "firewall.del":
		if r.RuleID < 1 {
			return nil, fmt.Errorf("rule number %d is not valid", r.RuleID)
		}
		return []string{"firewall", "remove", "-id", strconv.FormatInt(r.RuleID, 10), "-apply"}, nil
	case "port.status":
		return []string{"port", "status", "-json"}, nil
	case "port.sshaddr":
		return []string{"port", "ssh-addr"}, nil
	case "port.set":
		if err := svcport.Valid(r.SvcKey, r.Port); err != nil {
			return nil, err
		}
		return []string{"port", "set", "-service=" + r.SvcKey, "-port=" + strconv.Itoa(r.Port), "-apply"}, nil
	case "port.finish", "port.cancel":
		if !svcport.TwoStep(r.SvcKey) {
			return nil, fmt.Errorf("only a change of ssh or of the panel is finished or cancelled")
		}
		return []string{"port", strings.TrimPrefix(r.Op, "port."), "-service=" + r.SvcKey, "-apply"}, nil
	case "firewall.add":
		rule, err := firewall.ValidRule(firewall.Rule{Action: r.FwAction, Proto: r.FwProto, Port: r.FwPort, Source: r.FwSource, Comment: r.FwComment})
		if err != nil {
			return nil, err
		}
		if r.Client != "" && net.ParseIP(r.Client) == nil {
			return nil, fmt.Errorf("client address %q is not valid", r.Client)
		}
		// "=" joins each value to its flag, so a value that starts with "-" or is empty cannot be read as another flag.
		return []string{"firewall", "add", "-action=" + rule.Action, "-proto=" + rule.Proto, "-port=" + rule.Port, "-source=" + rule.Source,
			"-comment=" + rule.Comment, "-client=" + r.Client, "-apply"}, nil
	case "cloud.status":
		if err := optionalSite(r.Site); err != nil {
			return nil, err
		}
		a := []string{"cloud", "status", "-json"}
		if r.Site != "" {
			a = append(a, "-site="+r.Site)
		}
		return a, nil
	case "cloud.connect":
		if err := cloudsync.ValidClient(r.CloudClientID, r.CloudSecret); err != nil {
			return nil, err
		}
		return []string{"cloud", "connect", "-apply"}, nil
	case "cloud.configure":
		if r.CloudStorage == nil {
			return nil, fmt.Errorf("no storage settings")
		}
		// The helper checks again: a request that names a setting off the list of internal/cloudprov never reaches rclone.
		if _, err := r.CloudStorage.Check(); err != nil {
			return nil, err
		}
		return []string{"cloud", "configure", "-apply"}, nil
	case "cloud.finish":
		return []string{"cloud", "finish", "-apply"}, nil
	case "cloud.disconnect":
		return []string{"cloud", "disconnect", "-apply"}, nil
	case "cloud.enable":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"cloud", "enable", "-site=" + r.Site, "-on=" + strconv.FormatBool(r.CloudOn), "-apply"}, nil
	case "cloud.sync":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"cloud", "sync", "-site=" + r.Site, "-detach", "-apply"}, nil
	case "app.install":
		return r.installApp()
	case "mail.status", "mail.records", "mail.cfdetect", "mail.cfadd", "mail.enable", "mail.disable", "mail.ssl", "mail.webmail", "mail.domainadd", "mail.domaindel",
		"mail.boxadd", "mail.boxdel", "mail.boxpasswd", "mail.boxquota", "mail.aliasset", "mail.aliasdel":
		return r.mailArgv()
	case "wp.status", "wp.run", "wp.search":
		return r.wpArgv()
	case "cloud.list":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"cloud", "list", "-site=" + r.Site}, nil
	case "cloud.fetch":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		if !backup.ValidStamp(r.Backup) {
			return nil, fmt.Errorf("backup name %q is not valid", r.Backup)
		}
		return []string{"cloud", "fetch", "-site=" + r.Site, "-backup=" + r.Backup, "-detach", "-apply"}, nil
	case "panel.sslinfo":
		return []string{"panel", "ssl-info"}, nil
	case "panel.ssloff":
		return []string{"panel", "ssl-off", "-apply"}, nil
	case "panel.ssl", "panel.sslcustom":
		if err := panelssl.ValidDomain(r.Domain); err != nil {
			return nil, err
		}
		if r.Op == "panel.ssl" {
			return []string{"panel", "ssl", "-domain=" + r.Domain, "-apply"}, nil
		}
		for name, text := range map[string]string{"key": r.Key, "certificate": r.Cert, "chain": r.Chain} {
			if len(text) > maxCertText {
				return nil, fmt.Errorf("the %s is longer than %d KB", name, maxCertText>>10)
			}
			if strings.ContainsRune(text, 0) {
				return nil, fmt.Errorf("the %s has a character that cannot be part of a PEM text", name)
			}
		}
		if strings.TrimSpace(r.Cert) == "" || strings.TrimSpace(r.Key) == "" {
			return nil, fmt.Errorf("the certificate and its private key are both needed")
		}
		return []string{"panel", "ssl-custom", "-domain=" + r.Domain, "-apply"}, nil
	case "security.status":
		return []string{"security", "status", "-json"}, nil
	case "security.waf":
		if err := security.ValidMode(r.SecMode); err != nil {
			return nil, err
		}
		return []string{"security", "waf", "-mode=" + r.SecMode, "-apply"}, nil
	case "security.f2b":
		list, err := security.ValidIgnore(r.SecIgnore)
		if err != nil {
			return nil, err
		}
		return []string{"security", "f2b", "-on=" + strconv.FormatBool(r.SecOn), "-ignore=" + strings.Join(list, ","), "-apply"}, nil
	case "security.cloudflare":
		return []string{"security", "cloudflare", "-on=" + strconv.FormatBool(r.SecOn), "-apply"}, nil
	case "security.unban":
		if err := security.ValidJail(r.Jail); err != nil {
			return nil, err
		}
		ip, err := security.ValidAddr(r.IP)
		if err != nil {
			return nil, err
		}
		return []string{"security", "unban", "-jail=" + r.Jail, "-ip=" + ip, "-apply"}, nil
	case "site.cronlist", "site.cronadd", "site.cronset", "site.crondel":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		switch r.Op {
		case "site.cronlist":
			return []string{"cron", "list", "-site", r.Site, "-json"}, nil
		case "site.cronadd":
			if err := cron.ValidSchedule(r.Schedule); err != nil {
				return nil, err
			}
			if err := cron.ValidCommand(r.Command); err != nil {
				return nil, err
			}
			return []string{"cron", "add", "-site", r.Site, "-stdin", "-apply"}, nil
		}
		if r.JobID < 1 {
			return nil, fmt.Errorf("job number %d is not valid", r.JobID)
		}
		if r.Op == "site.crondel" {
			return []string{"cron", "remove", "-site", r.Site, "-id", strconv.FormatInt(r.JobID, 10), "-apply"}, nil
		}
		verb := "disable"
		if r.On {
			verb = "enable"
		}
		return []string{"cron", verb, "-site", r.Site, "-id", strconv.FormatInt(r.JobID, 10), "-apply"}, nil
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
	case "db.settingsinfo":
		return []string{"db", "settings"}, nil
	case "db.settings":
		if _, err := tune.CheckDBValues(r.DBValues); err != nil {
			return nil, err
		}
		return []string{"db", "settings", "-stdin", "-apply"}, nil // the values go on standard input
	case "db.remoteinfo":
		if err := requireDB(r.Name); err != nil {
			return nil, err
		}
		return []string{"db", "remote", "-name", r.Name, "-json"}, nil
	case "db.remote":
		if err := requireDB(r.Name); err != nil {
			return nil, err
		}
		hosts := "none"
		if len(r.Hosts) > 0 {
			list, err := dbadmin.ParseHosts(strings.Join(r.Hosts, ","))
			if err != nil {
				return nil, err
			}
			hosts = strings.Join(list, ",")
		}
		return []string{"db", "remote", "-name", r.Name, "-hosts", hosts, "-apply"}, nil
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
	case "backup.prune", "backup.keep":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		if r.Keep < 1 || r.Keep > 365 {
			return nil, fmt.Errorf("keep must be between 1 and 365")
		}
		if r.Op == "backup.keep" {
			return []string{"backup", "keep", "-site", r.Site, "-n", strconv.Itoa(r.Keep), "-apply"}, nil
		}
		return []string{"backup", "prune", "-site", r.Site, "-keep", strconv.Itoa(r.Keep), "-apply"}, nil
	case "backup.keepinfo":
		if err := requireSite(r.Site); err != nil {
			return nil, err
		}
		return []string{"backup", "keep", "-site", r.Site, "-json"}, nil
	case "notify.status", "notify.test", "notify.off", "notify.hub", "notify.chat", "notify.rules", "notify.noderules", "notify.pair", "notify.noderemove", "notify.agent":
		return r.notifyArgv()
	case "audit.list":
		return r.auditList()
	case "audit.note":
		// Written by the helper itself (see Server.Note); nothing is run.
		return nil, r.validNote()
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
		if r.CFToken != "" {
			return []string{"site", "ssl", "-name", r.Site, "-cf", "-apply"}, nil // the token goes on standard input
		}
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
		case r.Port != 0 && !site.ValidAppPort(r.Port):
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
		case r.Port != 0 && !site.ValidAppPort(r.Port):
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
