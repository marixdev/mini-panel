package installer

import (
	"errors"
	"fmt"
	"strings"

	"mpn/internal/site"
)

// PanelBackendPort is where the web panel listens on this machine. It is never reachable from outside: nginx takes the
// public port, speaks TLS, and passes requests on.
const PanelBackendPort = 8780

const (
	webMark   = "mpn web panel"
	panelCert = "/etc/mpn/certs/_panel"
)

const rootService = `[Unit]
Description=mpn root helper for the web panel
After=network.target

[Service]
ExecStart=` + mpnBinary + ` root
Restart=on-failure
RestartSec=2
RuntimeDirectory=mpn
RuntimeDirectoryMode=0755
RuntimeDirectoryPreserve=restart

[Install]
WantedBy=multi-user.target
`

// webService runs the panel as its own account with almost no rights: it cannot gain any, sees no home directories,
// writes nowhere, and may use only Unix sockets and IP. Everything it asks for goes through the root helper's socket.
func webService(hosts []string, pmaPort int) string {
	var h strings.Builder
	for _, x := range hosts {
		h.WriteString(" -host " + x)
	}
	if pmaPort > 0 {
		h.WriteString(fmt.Sprintf(" -pma-port %d", pmaPort))
	}
	return fmt.Sprintf(`[Unit]
Description=mpn web panel
After=mpn-root.service
Wants=mpn-root.service

[Service]
User=mpn-web
Group=mpn-web
ExecStart=%s web -listen 127.0.0.1:%d -secure -trust-proxy%s
Restart=on-failure
RestartSec=2
NoNewPrivileges=yes
CapabilityBoundingSet=
PrivateTmp=yes
PrivateDevices=yes
ProtectSystem=strict
ProtectHome=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=yes
LockPersonality=yes
SystemCallArchitectures=native
UMask=0077
MemoryMax=96M

[Install]
WantedBy=multi-user.target
`, mpnBinary, PanelBackendPort, h.String())
}

// panelNginx makes nginx answer the public port over TLS and hand requests to the panel. The Host header keeps its port
// (the panel compares it with the Origin of a form), and X-Real-IP is set here, replacing whatever a client sent.
func panelNginx(port int) string {
	return fmt.Sprintf(`server {
    listen %[1]d ssl http2;
    listen [::]:%[1]d ssl http2;
    server_name _;
    ssl_certificate %[2]s/fullchain.pem;
    ssl_certificate_key %[2]s/privkey.pem;
    client_max_body_size 1m;
    gzip on;
    gzip_types text/css text/javascript application/javascript;
    # What the database browser and nginx ask of the panel over the loopback address is not for anybody on the Internet.
    location ^~ /_pma/ { return 404; }
    # An upload is passed on as it arrives instead of being stored by nginx first, and may be large; the panel stops it at its own limit.
    location ~ ^/sites/[^/]+/files/upload$ {
        client_max_body_size %[4]dm;
        proxy_request_buffering off;
        client_body_timeout 2h;
        proxy_read_timeout 2h;
        proxy_send_timeout 2h;
        %[5]s
    }
    # The text of the code editor can be a few megabytes.
    location ~ ^/sites/[^/]+/files/save$ {
        client_max_body_size 8m;
        %[5]s
    }
    location / {
        %[5]s
        proxy_read_timeout 1500s;
        proxy_send_timeout 60s;
    }
}
`, port, panelCert, PanelBackendPort, UploadLimitMB, panelProxy(PanelBackendPort))
}

// UploadLimitMB is the largest upload nginx lets through to the panel (1 GiB, the panel's own default limit).
const UploadLimitMB = 1100

// panelProxy is what every location that hands a request to the panel has in common.
func panelProxy(backend int) string {
	return fmt.Sprintf(`proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto https;`, backend)
}

// sshdExemptScript keeps sshd from penalising this machine's own address. OpenSSH 9.8 and later slows down, then drops,
// connections from an address after failed logins, and every login of the panel comes from 127.0.0.1: one failed check
// would otherwise shut the terminal and the file manager of every website for seconds to minutes. Older OpenSSH has no
// such penalty and does not know the option, and a configuration with an unknown option stops sshd from starting, so
// the file is written only when sshd itself lists the option, and is removed again if sshd does not accept the result.
const sshdExemptScript = `S=$(command -v sshd || echo /usr/sbin/sshd); [ -x "$S" ] || exit 0
"$S" -T 2>/dev/null | grep -qi '^persourcepenaltyexemptlist' || exit 0
f=/etc/ssh/sshd_config.d/10-mpn-panel.conf
mkdir -p /etc/ssh/sshd_config.d
printf '# mpn: the web panel logs in from this machine, so this machine must not be penalised\nPerSourcePenaltyExemptList 127.0.0.1/32,::1/128\n' > "$f"
if "$S" -t 2>/dev/null; then systemctl reload ssh 2>/dev/null || systemctl reload sshd; else rm -f "$f"; echo 'sshd did not accept the exemption; removed it' >&2; fi`
const panelCertScript = `d=` + panelCert + `
[ -s "$d/fullchain.pem" ] || { mkdir -p "$d" && openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 3650 -subj /CN=mpn-panel -keyout "$d/privkey.pem" -out "$d/fullchain.pem" && chmod 600 "$d/privkey.pem"; }`

// webUserScript creates the panel's account, or accepts it again if mpn made it before. Any other account of that name
// stops the plan.
const webUserScript = `if id -u mpn-web >/dev/null 2>&1; then [ "$(getent passwd mpn-web | cut -d: -f5)" = '` + webMark + `' ] || { echo 'account mpn-web already exists and was not created by mpn' >&2; exit 1; }; else useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin --comment '` + webMark + `' mpn-web; fi`

// panelStep installs the web panel: its account, the two services, the TLS front on o.PanelPort, and the first
// administrator password, which is printed once if none exists yet.
func panelStep(o Options) (Step, error) {
	if o.SelfPath == "" {
		return Step{}, errors.New("the web panel needs the mpn binary installed; run the plan from the mpn binary itself")
	}
	port := o.PanelPort
	if port == 0 {
		port = 2610
	}
	if port < 1024 || port > 65535 || port == PanelBackendPort {
		return Step{}, fmt.Errorf("panel port %d must be between 1024 and 65535 and not %d", port, PanelBackendPort)
	}
	for _, h := range o.PanelHosts {
		if !site.ValidHostname(h) {
			return Step{}, fmt.Errorf("panel host name %q is not a valid lower-case ASCII name", h)
		}
	}
	acts := []Action{
		Run("sh", "-c", webUserScript),
		Run("install", "-d", "-o", "mpn-web", "-g", "mpn-web", "-m", "0750", "/var/lib/mpn-web"),
		WriteFile("/etc/systemd/system/mpn-root.service", rootService, 0o644),
		WriteFile("/etc/systemd/system/mpn-web.service", webService(o.PanelHosts, pmaPortOf(o, port)), 0o644),
		Run("sh", "-c", panelCertScript),
		Run("sh", "-c", sshdExemptScript),
		WriteFile("/etc/nginx/conf.d/01-mpn-panel.conf", panelNginx(port), 0o644),
		Run("nginx", "-t"),
		ReloadNginx(),
		Run("systemctl", "daemon-reload"),
		Run("systemctl", "enable", "mpn-root", "mpn-web"),
		Run("systemctl", "restart", "mpn-root"),
		Run("sh", "-c", "[ -s /var/lib/mpn-web/account.json ] || "+mpnBinary+" web passwd"),
		Run("systemctl", "restart", "mpn-web"),
	}
	if o.PMA { // the group and the key must exist before the panel starts with the browser switched on
		acts = append(acts[:2], append(pmaPrepareActions(), acts[2:]...)...)
	}
	return Step{"web panel", acts}, nil
}

// pmaPortOf is the port of the database browser for the web unit, 0 when it is not installed.
func pmaPortOf(o Options, panelPort int) int {
	if o.PMA {
		return PMAPort(panelPort)
	}
	return 0
}
