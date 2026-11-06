// Package vhost renders nginx server blocks from a validated site.
package vhost

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

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
}

// What a site serves. It appears once for HTTP and once for HTTPS when both serve it.
// The 443 listener uses "ssl http2" because the separate "http2 on;" directive needs nginx 1.25.1 or newer.
var tpl = template.Must(template.New("vhost").Parse(`{{define "body"}}
    root {{.Site.Root}};
    access_log /var/log/mpn/sites/{{.Site.Name}}/access.log;
    error_log /var/log/mpn/sites/{{.Site.Name}}/error.log;
    location ~ /\.(?!well-known) { deny all; }
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

// Render validates the site and returns the nginx config for it.
func Render(s site.Site, o Options) (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
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
	if err := tpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}
