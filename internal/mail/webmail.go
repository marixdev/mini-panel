package mail

import (
	"fmt"
	"regexp"
	"strings"

	"mpn/internal/installer"
	"mpn/internal/security"
)

// The web mail is mpn-mail, a single Go program (a customised lilmail). It runs under its own account, listens on the loopback only
// through a socket that systemd holds, exits when nobody uses it, and is reached from outside through nginx on a port of its own that
// shares the certificate of the panel, the way the database browser does. A port, not a path below the panel: the program writes
// absolute links, so it cannot live under /webmail.
const (
	WebmailBinary      = "/usr/local/bin/mpn-mail"
	WebmailDir         = "/var/lib/mpn-mail"
	WebmailUser        = "mpn-mail"
	WebmailBackendPort = 8781
	WebmailNginxFile   = "/etc/nginx/conf.d/03-mpn-webmail.conf"
	WebmailSocketUnit  = "/etc/systemd/system/mpn-mail.socket"
	WebmailServiceUnit = "/etc/systemd/system/mpn-mail.service"
	webmailTemplate    = "/usr/local/share/mpn/mail-config.toml.in"
	panelCertDir       = "/etc/mpn/certs/_panel"

	// IdleExitSeconds is how long the program waits with nobody using it before it exits and gives its memory back. systemd starts it
	// again on the next visit.
	IdleExitSeconds = 900
)

// The release the panel installs. The program is downloaded once, from the release of the project that publishes it, and compared with
// the checksum fixed here; a file that does not match is never installed.
const (
	WebmailVersion = "1.14.0-mpn.1"
	WebmailURL     = "https://github.com/marixdev/mpn-mail/releases/download/v" + WebmailVersion + "/mpn-mail_linux_amd64"
	WebmailSHA256  = "f82283355861cd29ffe8b8d4dd2a5b3b55c3a5d736786ac621751b476d37328d"
)

// WebmailPort is the port of the web mail: the second one after the panel's (the first is the database browser's).
func WebmailPort(panelPort int) int { return panelPort + 2 }

// WebmailConfig is config.toml with two places for the secrets, which are made once on the machine and kept when the file is
// written again, so that a change here does not sign everybody out. IMAP and SMTP are the servers of this machine, reached over the
// loopback: that is the one place Dovecot accepts a login without TLS.
func WebmailConfig() string {
	return "# Written by mpn. Do not edit; the panel rewrites this file and keeps the two secrets.\n" +
		"[server]\nport = " + fmt.Sprint(WebmailBackendPort) + "\nsecure_cookies = true\nlanguage = \"vi\"\nidle_exit_seconds = " + fmt.Sprint(IdleExitSeconds) + "\n\n" +
		"[auth]\nallow_full_email_username = true\n\n" +
		"[imap]\nserver = \"127.0.0.1\"\nport = 143\ntls = false\n\n" +
		"[smtp]\nserver = \"127.0.0.1\"\nport = 587\nuse_starttls = true\ninsecure_skip_verify = true\n\n" +
		"[cache]\nfolder = \"" + WebmailDir + "/cache\"\n\n" +
		"[jwt]\nsecret = \"@JWT@\"\n\n" +
		"[encryption]\nkey = \"@KEY@\"\n"
}

// configScript turns the template into config.toml. Secrets already in the file are reused; otherwise they are made. They are hex, so
// nothing in them can end a quote or a sed expression.
const configScript = `set -e
d=` + WebmailDir + `
cfg="$d/config.toml"
jwt=; key=
if [ -f "$cfg" ]; then
  jwt=$(sed -n 's/^secret = "\([0-9a-f]*\)"$/\1/p' "$cfg" | head -n1)
  key=$(sed -n 's/^key = "\([0-9a-f]*\)"$/\1/p' "$cfg" | head -n1)
fi
[ ${#jwt} -ge 32 ] || jwt=$(openssl rand -hex 32)
[ ${#key} -eq 32 ] || key=$(openssl rand -hex 16)
t=$(mktemp "$d/.config.XXXXXX")
sed -e "s/@JWT@/$jwt/" -e "s/@KEY@/$key/" ` + webmailTemplate + ` > "$t"
chown ` + WebmailUser + `:` + WebmailUser + ` "$t"; chmod 0600 "$t"
mv "$t" "$cfg"`

var localPathRe = regexp.MustCompile(`^/[A-Za-z0-9._/-]{1,200}$`)

// ValidLocalBinary accepts the path of a program already on this machine: absolute, plain characters, no "..".
func ValidLocalBinary(p string) error {
	if !localPathRe.MatchString(p) || strings.Contains(p, "..") {
		return fmt.Errorf("%q is not a plain absolute path", p)
	}
	return nil
}

// binaryScript puts the program in place. It does nothing when the checksum already matches, and checks the file against the fixed
// checksum before it installs anything. The file comes from local, when that is given, and from the release otherwise.
func binaryScript(local string) string {
	get := `curl -fsSL --retry 3 -o "$t" '` + WebmailURL + `' || { echo "webmail-download: the web mail program could not be downloaded from ` + WebmailURL + `" >&2; exit 1; }`
	if local != "" {
		get = `[ -f '` + local + `' ] || { echo "webmail-download: there is no file ` + local + `" >&2; exit 1; }
cp -- '` + local + `' "$t"`
	}
	return `set -e
dest=` + WebmailBinary + `
want=` + WebmailSHA256 + `
if [ -f "$dest" ] && [ "$(sha256sum "$dest" | cut -d' ' -f1)" = "$want" ]; then exit 0; fi
t=$(mktemp); trap 'rm -f "$t"' EXIT
` + get + `
[ "$(sha256sum "$t" | cut -d' ' -f1)" = "$want" ] || { echo "webmail-checksum: the file does not match the expected checksum; not installed" >&2; exit 1; }
install -m 0755 -o root -g root "$t" "$dest"`
}

// WebmailSocket is the socket systemd holds. It is on the loopback, so the program is never reachable from outside except through
// nginx.
func WebmailSocket() string {
	return "[Unit]\nDescription=mpn web mail socket\n\n[Socket]\nListenStream=127.0.0.1:" + fmt.Sprint(WebmailBackendPort) + "\n\n[Install]\nWantedBy=sockets.target\n"
}

// WebmailService runs the program as its own account with almost no rights, like the web panel. It has no [Install] section: the
// socket starts it.
func WebmailService() string {
	return `[Unit]
Description=mpn web mail
Requires=mpn-mail.socket
After=network.target dovecot.service

[Service]
User=` + WebmailUser + `
Group=` + WebmailUser + `
WorkingDirectory=` + WebmailDir + `
ExecStart=` + WebmailBinary + `
NoNewPrivileges=yes
CapabilityBoundingSet=
PrivateTmp=yes
PrivateDevices=yes
ProtectSystem=strict
ReadWritePaths=` + WebmailDir + `
ProtectHome=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=yes
LockPersonality=yes
SystemCallArchitectures=native
UMask=0077
MemoryMax=128M
`
}

// WebmailNginx is the server of the web mail on its own port. The program has its own sign-in, so nginx only passes requests on.
// Events from the server (new mail) are a long response, so buffering is off and the read timeout is long. The rules of the web
// application firewall are left out: they would stop mail that contains code, which is ordinary in mail.
func WebmailNginx(panelPort int) (string, error) {
	p := WebmailPort(panelPort)
	if panelPort < 1024 || p > 65535 || p == WebmailBackendPort {
		return "", fmt.Errorf("the web mail would need port %d: choose another panel port", p)
	}
	port := fmt.Sprint(p)
	var b strings.Builder
	b.WriteString("# Written by mpn. Do not edit.\n")
	b.WriteString("server {\n    listen " + port + " ssl http2;\n    listen [::]:" + port + " ssl http2;\n    server_name _;\n    " + security.OffInclude + "\n")
	b.WriteString("    ssl_certificate " + panelCertDir + "/fullchain.pem;\n    ssl_certificate_key " + panelCertDir + "/privkey.pem;\n")
	b.WriteString("    client_max_body_size 30m;\n    server_tokens off;\n")
	b.WriteString("    add_header X-Content-Type-Options nosniff always;\n    add_header Strict-Transport-Security \"max-age=31536000\" always;\n")
	b.WriteString("    location / {\n        proxy_pass http://127.0.0.1:" + fmt.Sprint(WebmailBackendPort) + ";\n")
	b.WriteString("        proxy_http_version 1.1;\n        # $http_host keeps the port. The program compares the Referer of a form (which has the port) with this name, and $host would\n        # drop the port, so every sending of a message from a port other than 443 would be refused as a forged request.\n        proxy_set_header Host $http_host;\n        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $remote_addr;\n        proxy_set_header X-Forwarded-Proto https;\n")
	b.WriteString("        proxy_buffering off;\n        proxy_read_timeout 3600s;\n        proxy_connect_timeout 60s;\n    }\n}\n")
	return b.String(), nil
}

// WebmailDomainFile is the server block that serves the web mail on the name of the mail host, on port 443, so its address needs no
// port. mpn panel sync writes it only while the web mail is on and the mail host has a certificate that covers its name.
const WebmailDomainFile = "/etc/nginx/conf.d/07-mpn-webmail-domain.conf"

// WebmailDomainNginx is that block. The program links to itself with absolute paths, so it cannot live under a path such as /mail
// of the panel's name; it gets the name of the mail host, which already has a certificate of its own (CertFile, KeyFile).
func WebmailDomainNginx(host string) (string, error) {
	if err := ValidDomain(host); err != nil {
		return "", fmt.Errorf("mail host: %w", err)
	}
	var b strings.Builder
	b.WriteString("# Written by mpn: the web mail on the name of the mail host. Do not edit.\n")
	b.WriteString("server {\n    listen 443 ssl http2;\n    listen [::]:443 ssl http2;\n    server_name " + host + ";\n    " + security.OffInclude + "\n")
	b.WriteString("    ssl_certificate " + CertFile + ";\n    ssl_certificate_key " + KeyFile + ";\n")
	b.WriteString("    client_max_body_size 30m;\n    server_tokens off;\n")
	b.WriteString("    add_header X-Content-Type-Options nosniff always;\n    add_header Strict-Transport-Security \"max-age=31536000\" always;\n")
	b.WriteString("    location / {\n        proxy_pass http://127.0.0.1:" + fmt.Sprint(WebmailBackendPort) + ";\n")
	b.WriteString("        proxy_http_version 1.1;\n        # $http_host is what the program compares with the Referer of a form.\n        proxy_set_header Host $http_host;\n        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $remote_addr;\n        proxy_set_header X-Forwarded-Proto https;\n")
	b.WriteString("        proxy_buffering off;\n        proxy_read_timeout 3600s;\n        proxy_connect_timeout 60s;\n    }\n}\n")
	return b.String(), nil
}

// WebmailPlan installs the web mail. panelPort is the port of the panel, which the web mail's port follows. It needs the panel (for
// its nginx and its certificate); the caller checks that.
func WebmailPlan(panelPort int, localBinary string) (installer.Plan, error) {
	conf, err := WebmailNginx(panelPort)
	if err != nil {
		return nil, err
	}
	if localBinary != "" {
		if err := ValidLocalBinary(localBinary); err != nil {
			return nil, err
		}
	}
	return installer.Plan{
		{Name: "web mail account and folders", Actions: []installer.Action{
			installer.Run("sh", "-c", "getent passwd "+WebmailUser+" >/dev/null || useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin "+WebmailUser),
			installer.Run("install", "-d", "-m", "0700", "-o", WebmailUser, "-g", WebmailUser, WebmailDir, WebmailDir+"/cache"),
			installer.Run("install", "-d", "-m", "0755", "-o", "root", "-g", "root", "/usr/local/share/mpn"),
		}},
		{Name: "web mail program", Actions: []installer.Action{installer.Run("sh", "-c", binaryScript(localBinary))}},
		{Name: "web mail configuration", Actions: []installer.Action{
			installer.WriteFile(webmailTemplate, WebmailConfig(), 0o644),
			installer.Run("sh", "-c", configScript),
			installer.WriteFile(WebmailSocketUnit, WebmailSocket(), 0o644),
			installer.WriteFile(WebmailServiceUnit, WebmailService(), 0o644),
			installer.Run("systemctl", "daemon-reload"),
			installer.WriteFile(WebmailNginxFile, conf, 0o644),
			installer.Run("nginx", "-t"),
			installer.ReloadNginx(),
		}},
		{Name: "start the web mail socket", Actions: []installer.Action{
			installer.Run("systemctl", "enable", "--now", "mpn-mail.socket"),
		}},
	}, nil
}

// WebmailOffPlan stops the socket and the program and takes the nginx server away. The account, the configuration and the sessions
// stay, so turning mail on again keeps everybody's settings.
func WebmailOffPlan() installer.Plan {
	return installer.Plan{{Name: "turn the web mail off", Actions: []installer.Action{
		installer.Run("sh", "-c", "systemctl disable --now mpn-mail.socket mpn-mail.service 2>/dev/null || true"),
		installer.Run("rm", "-f", WebmailNginxFile),
		installer.Run("nginx", "-t"),
		installer.ReloadNginx(),
	}}}
}
