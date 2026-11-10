package installer

import (
	"fmt"
	"strings"

	"mpn/internal/security"
)

// The panel on a plain https address. Once the panel has a name and a certificate for it (Let's Encrypt or the person's own), nginx
// also answers that name on port 443: the panel at the root and the database browser under /pma/, so no address needs ":2610".
// The ports stay for somebody who comes by the IP address.

const (
	// PanelDomainFile is the server block of that name. mpn panel sync writes it and takes it away again.
	PanelDomainFile = "/etc/nginx/conf.d/06-mpn-panel-domain.conf"
	// PMAPrefix is where the database browser lives on that name.
	PMAPrefix = "/pma/"
)

// pmaDomainLocations puts the database browser under /pma/ of the panel's own name. It is the same program as on its own port, run
// by the same pool and checked with the panel before every request, so a visitor without a session is sent to the sign-in page.
//
// It shares an origin with the panel here, which the port form does not. A flaw in phpMyAdmin that ran a script would run it on the
// panel's origin, so this part is closed in as far as a header can: the page may only talk to /pma/ (connect-src, form-action),
// cannot be framed, and cannot be reached from a window of the panel (Cross-Origin-Opener-Policy).
func pmaDomainLocations() string {
	php := `include fastcgi_params;
            fastcgi_param SCRIPT_FILENAME $request_filename;
            fastcgi_param HTTPS on;
            fastcgi_pass unix:/run/php/mpn-pma.sock;
            fastcgi_read_timeout 300s;`
	var b strings.Builder
	b.WriteString(`    # Is this visitor signed in to the panel? The panel answers 204 or 401 from the cookie it sees.
    location = /_mpn_auth {
        internal;
        proxy_pass http://127.0.0.1:` + fmt.Sprint(PanelBackendPort) + `/_pma/auth;
        proxy_pass_request_body off;
        proxy_set_header Content-Length "";
        proxy_set_header Host localhost;
        proxy_set_header Cookie $http_cookie;
    }
    location @mpn_login { return 302 https://$host/login; }

    # phpMyAdmin under /pma/.
    location ^~ ` + PMAPrefix + ` {
        auth_request /_mpn_auth;
        error_page 401 = @mpn_login;
        alias ` + pmaDir + `/;
        index index.php;
        client_max_body_size 64m;
        add_header X-Content-Type-Options nosniff always;
        add_header Referrer-Policy same-origin always;
        add_header Cache-Control "no-store" always;
        add_header Cross-Origin-Opener-Policy same-origin always;
        add_header Content-Security-Policy "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; connect-src https://$http_host` + PMAPrefix + `; form-action https://$http_host` + PMAPrefix + `; frame-ancestors 'none'; base-uri 'self'" always;

        # The program's own folders, and files that are not for a browser.
        location ~ ^/pma/(libraries|templates|vendor|sql|locale|tmp|config|setup|doc|examples|test|node_modules)/ { return 404; }
        location ~ /\.(?!well-known) { return 404; }
        location ~* \.(inc|sh|md|lock|log|ini|dist|yml|yaml|xml|neon|twig|tpl|json|map)$ { return 404; }
        location ~ ^/pma/(README|ChangeLog|LICENSE|RELEASE-DATE.*|robots\.txt)$ { return 404; }
        # PHP is run only for the files it should run.
        location ~ ^/pma/((index|signon|logout|url|js/messages)\.php)$ {
            alias ` + pmaDir + `/$1;
            ` + php + `
        }
        location ~ \.php$ { return 404; }
    }
`)
	return b.String()
}

// PanelDomainNginx is the server block of the panel's name on port 443. pma says whether the database browser is installed.
func PanelDomainNginx(domain string, pma bool) string {
	var b strings.Builder
	b.WriteString("# Written by mpn: the panel on the plain https address of its name. Do not edit.\n")
	b.WriteString("server {\n    listen 443 ssl http2;\n    listen [::]:443 ssl http2;\n    server_name " + domain + ";\n")
	b.WriteString("    ssl_certificate " + panelCert + "/fullchain.pem;\n    ssl_certificate_key " + panelCert + "/privkey.pem;\n")
	b.WriteString("    client_max_body_size 1m;\n    gzip on;\n    gzip_types text/css text/javascript application/javascript;\n")
	b.WriteString("    " + security.OffInclude + "\n")
	b.WriteString("    # What the database browser and nginx ask of the panel over the loopback address is not for anybody on the Internet.\n")
	b.WriteString("    location ^~ /_pma/ { return 404; }\n")
	if pma {
		b.WriteString(pmaDomainLocations())
	}
	b.WriteString(fmt.Sprintf(`    # An upload is passed on as it arrives instead of being stored by nginx first, and may be large; the panel stops it at its own limit.
    location ~ ^/sites/[^/]+/files/upload$ {
        client_max_body_size %[1]dm;
        proxy_request_buffering off;
        client_body_timeout 2h;
        proxy_read_timeout 2h;
        proxy_send_timeout 2h;
        %[2]s
    }
    # The text of the code editor can be a few megabytes.
    location ~ ^/sites/[^/]+/files/save$ {
        client_max_body_size 8m;
        %[2]s
    }
    location / {
        %[2]s
        proxy_read_timeout 1500s;
        proxy_send_timeout 60s;
    }
}
`, UploadLimitMB, panelProxy(PanelBackendPort)))
	return b.String()
}
