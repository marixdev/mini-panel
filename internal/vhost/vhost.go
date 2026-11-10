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

type data struct {
	Site      site.Site
	Names     string // the names that serve the site
	Aliases   string // the names that only send visitors to the primary one
	Primary   string
	Scheme    string // of the address visitors are sent to
	Upstream  string
	TLS       bool
	AllowHTTP bool
	CertDir   string
	BodyMax   string // client_max_body_size of a PHP site: what PHP accepts, so the two agree
	Timeout   int    // how long nginx waits for PHP, in seconds
	Webmail   string // the locations of the web mail, when the site has it and serves over SSL
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
{{- template "body" .}}
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
    return 301 https://{{.Primary}}$request_uri;
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

// Render validates the site and returns the nginx config for it: the one a person wrote, when there is one.
func Render(s site.Site, o Options) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	if s.CustomVhost != "" {
		if err := CheckCustom(s.CustomVhost, o.TLS); err != nil {
			return "", err
		}
		return strings.TrimRight(s.CustomVhost, "\r\n") + "\n", nil
	}
	if o.TLS && !site.SafePath(o.CertDir) {
		return "", fmt.Errorf("certificate directory %q is not a safe path", o.CertDir)
	}
	var b bytes.Buffer
	d := data{Site: s, Names: strings.Join(s.Hostnames, " "), Primary: s.Primary(), Upstream: s.ProxyTarget(), TLS: o.TLS, AllowHTTP: s.AllowHTTP, CertDir: o.CertDir, Scheme: "http"}
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
