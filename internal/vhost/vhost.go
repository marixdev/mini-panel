// Package vhost renders nginx server blocks from a validated site.
package vhost

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"unicode/utf8"

	"mpn/internal/site"
)

// Options controls how a site is served. With TLS, port 80 only answers ACME challenges and redirects, unless the site allows
// plain HTTP.
type Options struct {
	TLS     bool
	CertDir string // directory holding fullchain.pem and privkey.pem
}

// DefaultCertDir is the self-signed certificate the installer makes for the catch-all server. A site without a certificate of
// its own answers https:// with it and sends the visitor to http://, so the visitor sees the site and not the page of the
// catch-all (which nginx would otherwise pick, since no server of port 443 has the name).
const DefaultCertDir = "/etc/mpn/certs/_default"

type data struct {
	Site        site.Site
	Names       string // the names that serve the site
	Aliases     string // the names that only send visitors to the primary one
	Primary     string
	Scheme      string // of the address visitors are sent to
	Upstream    string
	TLS         bool
	AllowHTTP   bool
	CertDir     string
	BodyMax     string // client_max_body_size of a PHP site: what PHP accepts, so the two agree
	Timeout     int    // how long nginx waits for PHP, in seconds
	Webmail     string // the locations of the web mail, when the site has it and serves over SSL
	AllNames    string // every name of the site, for the 443 server of a site without a certificate
	DefaultCert string
}

// What a site serves. It appears once for HTTP and once for HTTPS when both serve it.
// The 443 listener uses "ssl http2" because the separate "http2 on;" directive needs nginx 1.25.1 or newer.
var tpl = template.Must(template.New("vhost").Parse(`{{define "body"}}
    root {{.Site.Root}};
    access_log /var/log/mpn/sites/{{.Site.Name}}/access.log;
    error_log /var/log/mpn/sites/{{.Site.Name}}/error.log;
    location ~ /\.(?!well-known) { deny all; }
{{- if .Site.Speed}}
    pagespeed on;
    pagespeed FileCachePath /var/cache/ngx_pagespeed;
{{- end}}
{{- if .Webmail}}
{{.Webmail}}
{{- end}}
{{- if eq .Site.Kind "static"}}
    index index.html;
    location / { try_files $uri $uri/ =404; }
{{- else if and (eq .Site.Kind "php") (eq .Site.App "phpbb")}}
    index index.php index.html index.htm;
    client_max_body_size {{.BodyMax}};
{{- if not .Site.AppReady}}
    # The installer of phpBB. mpn removes this block and the install folder once an administrator exists.
    location ^~ /install/ {
        try_files $uri $uri/ @rewrite_installapp;
        location ~ \.php(/|$) {
{{- template "phpbbfcgi" .}}
        }
    }
    location @rewrite_installapp { rewrite ^(.*)$ /install/app.php/$1 last; }
{{- end}}
    # The internal folders and files of phpBB.
    location ~ /(config|common\.php|composer\.(json|lock)|cache|files|images/avatars/upload|includes|(?<!ext/)phpbb(?!\w+)|store|vendor|vendor-ext) {
        deny all;
    }
    # PHP with PATH_INFO, as in /app.php/feed.
    location ~ \.php(/|$) {
{{- template "phpbbfcgi" .}}
    }
    location / { try_files $uri $uri/ @rewriteapp; }
    location @rewriteapp { rewrite ^(.*)$ /index.php/$1 last; }
{{- else if eq .Site.Kind "php"}}
    index index.php index.html;
    client_max_body_size {{.BodyMax}};
    location / { try_files $uri $uri/ /index.php?$query_string; }
    location ~ \.php$ {
        try_files $uri =404;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        fastcgi_pass unix:/run/php/{{.Site.Name}}.sock;
        fastcgi_read_timeout {{.Timeout}}s;
    }
{{- else}}
    location / {
        proxy_pass {{.Upstream}};
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
{{- end}}
{{- end}}
{{define "phpbbfcgi"}}
            include fastcgi_params;
            fastcgi_split_path_info ^(.+\.php)(/.*)$;
            fastcgi_param PATH_INFO $fastcgi_path_info;
            fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
            fastcgi_param DOCUMENT_ROOT $realpath_root;
            try_files $fastcgi_script_name =404;
            fastcgi_pass unix:/run/php/{{.Site.Name}}.sock;
            fastcgi_read_timeout {{.Timeout}}s;
{{- end}}
{{define "ssl"}}
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    ssl_certificate {{.CertDir}}/fullchain.pem;
    ssl_certificate_key {{.CertDir}}/privkey.pem;
{{- end}}
server {
    listen 80;
    listen [::]:80;
    server_name {{.Names}};
    location /.well-known/acme-challenge/ { root /var/lib/mpn/acme; }
{{- if and .TLS (not .AllowHTTP)}}
    location / { return 301 https://$host$request_uri; }
}
{{- else}}
{{- template "body" .}}
}
{{- end}}
{{- if .TLS}}

server {
{{- template "ssl" .}}
    server_name {{.Names}};
    # Behind a proxy that reaches this machine over https (Cloudflare in "Full" mode), the renewal asks here.
    location /.well-known/acme-challenge/ { root /var/lib/mpn/acme; }
{{- template "body" .}}
}
{{- end}}
{{- if not .TLS}}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    ssl_certificate {{.DefaultCert}}/fullchain.pem;
    ssl_certificate_key {{.DefaultCert}}/privkey.pem;
    server_name {{.AllNames}};
    location /.well-known/acme-challenge/ { root /var/lib/mpn/acme; }
    location / { return 302 http://$host$request_uri; }
}
{{- end}}
{{- if .Aliases}}

server {
    listen 80;
    listen [::]:80;
    server_name {{.Aliases}};
    location /.well-known/acme-challenge/ { root /var/lib/mpn/acme; }
    location / { return 301 {{.Scheme}}://{{.Primary}}$request_uri; }
}
{{- if .TLS}}

server {
{{- template "ssl" .}}
    server_name {{.Aliases}};
    location /.well-known/acme-challenge/ { root /var/lib/mpn/acme; }
    location / { return 301 https://{{.Primary}}$request_uri; }
}
{{- end}}
{{- end}}
`))

// MaxCustom is the longest configuration a person may write by hand. It has to fit in one request to the root helper.
const MaxCustom = 48 << 10

var serverBlockRe = regexp.MustCompile(`(?m)^\s*server\s*\{`)

// CheckCustom says what is wrong with a configuration written by hand, or nil. nginx -t decides whether nginx accepts it; this
// catches what nginx cannot know: that it is a server block at all, and that the renewal of a certificate would still work.
func CheckCustom(text string, ssl bool) error {
	switch {
	case strings.TrimSpace(text) == "":
		return fmt.Errorf("the configuration is empty")
	case len(text) > MaxCustom:
		return fmt.Errorf("the configuration is longer than %d KB", MaxCustom>>10)
	case !utf8.ValidString(text) || strings.ContainsRune(text, 0):
		return fmt.Errorf("the configuration has characters that cannot be part of a text file")
	case !serverBlockRe.MatchString(text):
		return fmt.Errorf("the configuration has no server { } block")
	case ssl && !strings.Contains(text, "/.well-known/acme-challenge/"):
		return fmt.Errorf("the location /.well-known/acme-challenge/ is missing: without it the certificate cannot be renewed")
	}
	return nil
}

// CheckCustomFor adds the checks that depend on the site. nginx opens its log files as root before it drops to its worker
// account, so a log line of a hand-written configuration could make root create or append to any file, such as one in /etc,
// and include, load_module and the like could read files of other sites or load code. Log files must therefore be in the
// site's own log folder, and the directives that reach other files or the whole server are refused.
func CheckCustomFor(text, siteName string, ssl bool) error {
	if err := CheckCustom(text, ssl); err != nil {
		return err
	}
	dir := "/var/log/mpn/sites/" + siteName + "/"
	for i, line := range strings.Split(stripComments(text), "\n") {
		// A statement ends at ";", and a block opens or closes at "{" and "}": each of them starts a new statement, so the
		// directive inside "location / { access_log ...; }" is read as a directive too.
		for _, stmt := range strings.FieldsFunc(line, func(r rune) bool { return r == ';' || r == '{' || r == '}' }) {
			f := strings.Fields(stmt)
			for j := 0; j < len(f); j++ {
				name := f[j]
				switch name {
				case "include", "load_module", "ssl_certificate", "ssl_certificate_key", "ssl_trusted_certificate", "ssl_client_certificate",
					"ssl_dhparam", "ssl_crl", "ssl_password_file", "ssl_stapling_file", "ssl_session_ticket_key", "auth_basic_user_file",
					"lua_package_path", "perl_modules", "perl_require", "js_import", "js_path", "pid", "lock_file", "user", "working_directory",
					"client_body_temp_path", "proxy_temp_path", "fastcgi_temp_path", "uwsgi_temp_path", "scgi_temp_path",
					"proxy_cache_path", "fastcgi_cache_path", "uwsgi_cache_path", "scgi_cache_path":
					if !allowedPathDirective(name, f[j+1:], siteName) {
						return fmt.Errorf("line %d: %s is not allowed in a configuration written by hand", i+1, name)
					}
				case "access_log", "error_log":
					if j+1 >= len(f) {
						break
					}
					p := strings.Trim(f[j+1], `"'`)
					if p == "off" || p == "stderr" || strings.HasPrefix(p, "syslog:") || strings.HasPrefix(p, "memory:") {
						break
					}
					if strings.ContainsAny(p, "$") || !strings.HasPrefix(p, dir) || strings.Contains(p[len(dir):], "/") || strings.Contains(p, "..") {
						return fmt.Errorf("line %d: %s must write to a file in %s (it is %s)", i+1, name, dir, p)
					}
				}
				break // only the first word of a statement is its directive
			}
		}
	}
	return nil
}

// allowedPathDirective lets through the few uses of those directives the panel's own configuration has: the certificate of the
// site that mpn manages, and the files mpn ships for every site.
func allowedPathDirective(name string, args []string, siteName string) bool {
	if len(args) == 0 {
		return false
	}
	p := strings.Trim(args[0], `"'`)
	if strings.Contains(p, "..") || strings.Contains(p, "$") {
		return false
	}
	switch name {
	case "ssl_certificate", "ssl_certificate_key":
		return strings.HasPrefix(p, "/etc/letsencrypt/live/") || strings.HasPrefix(p, "/etc/mpn/certs/"+siteName+"/") || strings.HasPrefix(p, DefaultCertDir+"/")
	case "include":
		// The files of nginx and of mpn that the configurations mpn writes include: fastcgi and proxy parameters, the switch of the
		// firewall and the default page.
		switch p {
		case "fastcgi_params", "/etc/nginx/fastcgi_params", "proxy_params", "/etc/nginx/proxy_params", "/etc/nginx/snippets/fastcgi-php.conf",
			"/etc/mpn/modsec/off.d/*.conf", "/etc/nginx/mpn-default-page.conf":
			return true
		}
	}
	return false
}

// stripComments removes what follows a # on each line, outside quotes.
func stripComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		q := byte(0)
		cut := len(line)
		for i := 0; i < len(line); i++ {
			c := line[i]
			switch {
			case q != 0 && c == q:
				q = 0
			case q == 0 && (c == '"' || c == '\''):
				q = c
			case q == 0 && c == '#':
				cut = i
				i = len(line)
			}
		}
		b.WriteString(line[:cut])
		b.WriteByte('\n')
	}
	return b.String()
}

// Render validates the site and returns the nginx config for it: the one a person wrote, when there is one.
func Render(s site.Site, o Options) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	if s.CustomVhost != "" {
		// Checked again here, so a configuration saved by an older mpn is held to the same rules before nginx loads it as root.
		if err := CheckCustomFor(s.CustomVhost, s.Name, o.TLS); err != nil {
			return "", err
		}
		return strings.TrimRight(s.CustomVhost, "\r\n") + "\n", nil
	}
	if o.TLS && !site.SafePath(o.CertDir) {
		return "", fmt.Errorf("certificate directory %q is not a safe path", o.CertDir)
	}
	var b bytes.Buffer
	d := data{Site: s, Names: strings.Join(s.Hostnames, " "), Primary: s.Primary(), Upstream: s.ProxyTarget(), TLS: o.TLS, AllowHTTP: s.AllowHTTP, CertDir: o.CertDir, Scheme: "http",
		AllNames: strings.Join(s.Hostnames, " "), DefaultCert: DefaultCertDir}
	if o.TLS {
		d.Scheme = "https"
	}
	if aliases := s.Aliases(); len(aliases) > 0 {
		// With a www choice the first name serves the site and the others only send visitors to it.
		d.Names, d.Aliases = s.Primary(), strings.Join(aliases, " ")
	}
	if s.Kind == site.PHP {
		d.BodyMax, d.Timeout = s.BodyLimit(), s.RequestSeconds()
	}
	if s.Webmail && o.TLS {
		d.Webmail = strings.TrimRight(WebmailLocations(), "\n")
	}
	if err := tpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}
