package installer

import (
	"fmt"
	"slices"
	"strings"

	"mpn/internal/aptsrc"
	"mpn/internal/site"
)

// The database browser: phpMyAdmin, installed from the release of its own site (checked against a hash that is fixed in
// this file), run by its own PHP-FPM pool under its own account, on a port of its own, and reachable only by somebody who
// has a signed-in panel session. See internal/web/pma.go for how a visit begins.

const (
	pmaVersion = "5.2.3"
	// pmaSHA256 is the hash published with the release, and was checked against the downloaded file.
	pmaSHA256 = "57881348297c4412f86c410547cf76b4d8a236574dd2c6b7d6a2beebe7fc44e3"
	pmaURL    = "https://files.phpmyadmin.net/phpMyAdmin/" + pmaVersion + "/phpMyAdmin-" + pmaVersion + "-all-languages.tar.xz"
	// pmaPHP is the PHP version it runs on: 5.2 supports up to 8.3.
	pmaPHP = "8.3"

	pmaDir     = "/usr/share/mpn/phpmyadmin"
	pmaConf    = "/etc/mpn/pma"
	pmaState   = "/var/lib/mpn-pma"
	pmaAccount = "mpn-pma"
	pmaMark    = "mpn phpmyadmin"
)

// PMAPort is the port of the database browser: the one after the panel's.
func PMAPort(panelPort int) int { return panelPort + 1 }

// pmaUserScript creates the browser's account and the group the panel's web account shares with it, so that both can read
// the key and nothing else is shared. An account of that name that mpn did not make stops the plan.
const pmaUserScript = `if id -u ` + pmaAccount + ` >/dev/null 2>&1; then [ "$(getent passwd ` + pmaAccount + ` | cut -d: -f5)" = '` + pmaMark + `' ] || { echo 'account ` + pmaAccount + ` already exists and was not created by mpn' >&2; exit 1; }; else useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin --comment '` + pmaMark + `' ` + pmaAccount + `; fi
usermod -aG ` + pmaAccount + ` mpn-web
install -d -m 0755 /etc/mpn
install -d -o root -g ` + pmaAccount + ` -m 0750 ` + pmaConf + `
[ -s ` + pmaConf + `/secret ] || { head -c 64 /dev/urandom | base64 -w0 | tr -d '/+=' | cut -c1-48 > ` + pmaConf + `/secret; }
chown root:` + pmaAccount + ` ` + pmaConf + `/secret; chmod 0640 ` + pmaConf + `/secret`

// pmaPrepareActions are what the web panel needs before it starts with the browser switched on: the group and the key.
func pmaPrepareActions() []Action { return []Action{Run("sh", "-c", pmaUserScript)} }

// pmaInstallScript puts the release in place. It does nothing when this version is already there, checks the hash before
// it unpacks anything, and swaps the folder in one step so a failed download leaves the old one working.
const pmaInstallScript = `set -e
d=` + pmaDir + `
[ "$(cat "$d/.mpn-version" 2>/dev/null)" = '` + pmaVersion + `' ] && exit 0
t=$(mktemp -d); trap 'rm -rf "$t"' EXIT
curl -fsSL --retry 3 -o "$t/pma.tar.xz" '` + pmaURL + `'
echo '` + pmaSHA256 + `  '"$t/pma.tar.xz" | sha256sum -c - >/dev/null || { echo 'phpMyAdmin download does not match the expected hash; not installed' >&2; exit 1; }
mkdir "$t/x"; tar -xJf "$t/pma.tar.xz" -C "$t/x" --strip-components=1
# the setup wizard, the examples and the docs are of no use here, and the wizard is a way in
rm -rf "$t/x/setup" "$t/x/examples" "$t/x/doc" "$t/x/test" "$t/x/composer.json" "$t/x/composer.lock" "$t/x/package.json" "$t/x/yarn.lock"
echo '` + pmaVersion + `' > "$t/x/.mpn-version"
install -d -m 0755 "$(dirname "$d")"
rm -rf "$d.old"; [ -e "$d" ] && mv "$d" "$d.old"; mv "$t/x" "$d"; rm -rf "$d.old"
chown -R root:root "$d"; find "$d" -type d -exec chmod 755 {} +; find "$d" -type f -exec chmod 644 {} +`

// pmaStateScript makes the folders the browser writes to: its sessions and its temporary files, for its account alone.
const pmaStateScript = `install -d -o ` + pmaAccount + ` -g ` + pmaAccount + ` -m 0700 ` + pmaState + ` ` + pmaState + `/sessions ` + pmaState + `/tmp`

func pmaPool() string {
	return `[mpn-pma]
user = ` + pmaAccount + `
group = ` + pmaAccount + `
listen = /run/php/mpn-pma.sock
listen.owner = www-data
listen.group = www-data
pm = ondemand
pm.max_children = 3
pm.process_idle_timeout = 20s
pm.max_requests = 200
request_terminate_timeout = 300s
php_admin_value[memory_limit] = 192M
php_admin_value[upload_max_filesize] = 64M
php_admin_value[post_max_size] = 64M
php_admin_value[max_execution_time] = 300
php_admin_value[open_basedir] = ` + pmaDir + `:` + pmaState + `:` + pmaConf + `:/usr/share/php:/usr/share/javascript
php_admin_value[session.save_path] = ` + pmaState + `/sessions
php_admin_value[upload_tmp_dir] = ` + pmaState + `/tmp
php_admin_value[sys_temp_dir] = ` + pmaState + `/tmp
php_admin_value[disable_functions] = exec,passthru,shell_exec,system,proc_open,popen,pcntl_exec,mail
php_admin_value[allow_url_fopen] = off
php_admin_value[allow_url_include] = off
php_admin_value[expose_php] = off
php_admin_value[session.use_strict_mode] = 1
php_admin_value[session.cookie_httponly] = 1
php_admin_value[session.gc_maxlifetime] = 1800
`
}

// pmaConfig is phpMyAdmin's configuration. Signing in is not done here: the sign-on page of this very folder starts the
// session from a ticket. The Server entry allows no arbitrary host, no root, and no empty password.
func pmaConfig() string {
	return `<?php
// Written by mpn. Changes are overwritten when the plan is applied again.
$cfg['blowfish_secret'] = substr(trim((string) @file_get_contents('` + pmaConf + `/secret')), 0, 32);
$i = 1;
$cfg['Servers'][$i]['auth_type'] = 'signon';
$cfg['Servers'][$i]['SignonSession'] = 'MpnPmaSignon';
$cfg['Servers'][$i]['SignonCookieParams'] = ['lifetime' => 0, 'path' => '/', 'secure' => true, 'httponly' => true, 'samesite' => 'Lax'];
$cfg['Servers'][$i]['SignonURL'] = 'signon.php';
$cfg['Servers'][$i]['LogoutURL'] = 'logout.php';
$cfg['Servers'][$i]['host'] = 'localhost';
$cfg['Servers'][$i]['compress'] = false;
$cfg['Servers'][$i]['AllowNoPassword'] = false;
$cfg['Servers'][$i]['AllowRoot'] = false;
$cfg['Servers'][$i]['hide_db'] = '^(information_schema|performance_schema|mysql|sys)$';
$cfg['AllowArbitraryServer'] = false;
$cfg['TempDir'] = '` + pmaState + `/tmp';
$cfg['UploadDir'] = '';
$cfg['SaveDir'] = '';
$cfg['ShowChgPassword'] = false;
$cfg['VersionCheck'] = false;
$cfg['SendErrorReports'] = 'never';
$cfg['CheckConfigurationPermissions'] = false;
$cfg['PmaNoRelation_DisableWarning'] = true;
$cfg['ExecTimeLimit'] = 300;
$cfg['DefaultLang'] = 'vi';
`
}

// pmaSignon is the page a visit begins at. It takes the ticket from the address, trades it for the temporary user at the
// panel (over the loopback address, with the key), and starts the session phpMyAdmin reads. Without a valid ticket it
// shows a message and nothing else.
const pmaSignon = `<?php
declare(strict_types=1);
header('Cache-Control: no-store');
header('Referrer-Policy: same-origin');
function stop(string $message, int $code): void {
    http_response_code($code);
    header('Content-Type: text/plain; charset=utf-8');
    echo $message;
    exit;
}
$t = $_GET['t'] ?? '';
if (!is_string($t) || !preg_match('/^[A-Za-z0-9_-]{43}$/', $t)) {
    stop("Hãy mở phpMyAdmin từ bảng điều khiển: nút phpMyAdmin ở danh sách cơ sở dữ liệu.", 403);
}
$key = trim((string) @file_get_contents('` + pmaConf + `/secret'));
if (strlen($key) < 32) {
    stop('phpMyAdmin chưa được cấu hình xong.', 500);
}
$ch = curl_init('http://127.0.0.1:@BACKEND@/_pma/ticket?t=' . $t);
curl_setopt_array($ch, [CURLOPT_RETURNTRANSFER => true, CURLOPT_TIMEOUT => 5, CURLOPT_HTTPHEADER => ['X-Mpn-Key: ' . $key, 'Host: localhost']]);
$body = curl_exec($ch);
$code = (int) curl_getinfo($ch, CURLINFO_HTTP_CODE);
curl_close($ch);
if ($code !== 200 || !is_string($body)) {
    stop("Liên kết này đã được dùng hoặc đã hết hạn. Hãy mở lại phpMyAdmin từ bảng điều khiển.", 403);
}
$d = json_decode($body, true);
if (!is_array($d) || !preg_match('/^pma_[0-9a-z]{6,8}_[0-9a-z]{8}$/', (string) ($d['user'] ?? ''))
    || !is_string($d['password'] ?? null) || !preg_match('/^[a-z0-9_]{1,32}$/', (string) ($d['database'] ?? ''))) {
    stop('Bảng điều khiển trả lời không đúng.', 502);
}
$params = ['lifetime' => 0, 'path' => '/', 'secure' => true, 'httponly' => true, 'samesite' => 'Lax'];
session_name('MpnPmaSignon');
session_set_cookie_params($params);
session_start();
session_regenerate_id(true);
$_SESSION['PMA_single_signon_user'] = $d['user'];
$_SESSION['PMA_single_signon_password'] = $d['password'];
$_SESSION['PMA_single_signon_host'] = 'localhost';
session_write_close();
// So that signing out can drop the user even if phpMyAdmin has already cleared its session. (setcookie takes "expires",
// not the "lifetime" of the session parameters; PHP 8 refuses an array with a key it does not know.)
setcookie('MpnPmaUser', $d['user'], ['expires' => 0, 'path' => '/', 'secure' => true, 'httponly' => true, 'samesite' => 'Lax']);
header('Location: index.php?route=/database/structure&db=' . rawurlencode($d['database']));
`

// pmaLogout ends a visit: it drops the temporary user at the panel, forgets the session, and goes back to the panel.
const pmaLogout = `<?php
declare(strict_types=1);
header('Cache-Control: no-store');
$u = $_COOKIE['MpnPmaUser'] ?? '';
$key = trim((string) @file_get_contents('` + pmaConf + `/secret'));
if (is_string($u) && preg_match('/^pma_[0-9a-z]{6,8}_[0-9a-z]{8}$/', $u) && strlen($key) >= 32) {
    $ch = curl_init('http://127.0.0.1:@BACKEND@/_pma/end?u=' . $u);
    curl_setopt_array($ch, [CURLOPT_RETURNTRANSFER => true, CURLOPT_TIMEOUT => 5, CURLOPT_HTTPHEADER => ['X-Mpn-Key: ' . $key, 'Host: localhost']]);
    curl_exec($ch);
    curl_close($ch);
}
$params = ['lifetime' => 0, 'path' => '/', 'secure' => true, 'httponly' => true, 'samesite' => 'Lax'];
session_name('MpnPmaSignon');
session_set_cookie_params($params);
session_start();
$_SESSION = [];
session_destroy();
setcookie('MpnPmaUser', '', ['expires' => 1, 'path' => '/', 'secure' => true, 'httponly' => true, 'samesite' => 'Lax']);
$host = (string) ($_SERVER['SERVER_NAME'] ?? 'localhost');
if (!preg_match('/^[A-Za-z0-9.\-]{1,253}$/', $host)) {
    $host = 'localhost';
}
header('Location: https://' . $host . ':@PANELPORT@/databases');
`

// pmaNginx is the server of the browser. Every request is checked with the panel first (auth_request), PHP is run only
// for the files it should run, and the folders of the program itself are closed. A visitor without a session is sent to
// the panel's login page.
func pmaNginx(panelPort int) string {
	pp := fmt.Sprint(PMAPort(panelPort))
	return `server {
    listen ` + pp + ` ssl http2;
    listen [::]:` + pp + ` ssl http2;
    server_name _;
    ssl_certificate ` + panelCert + `/fullchain.pem;
    ssl_certificate_key ` + panelCert + `/privkey.pem;
    root ` + pmaDir + `;
    index index.php;
    client_max_body_size 64m;
    add_header X-Content-Type-Options nosniff always;
    add_header Referrer-Policy same-origin always;
    add_header Strict-Transport-Security "max-age=31536000" always;
    add_header Cache-Control "no-store" always;
    server_tokens off;

    # Is this visitor signed in to the panel? The panel answers 204 or 401 from the cookie it sees.
    location = /_mpn_auth {
        internal;
        proxy_pass http://127.0.0.1:` + fmt.Sprint(PanelBackendPort) + `/_pma/auth;
        proxy_pass_request_body off;
        proxy_set_header Content-Length "";
        proxy_set_header Host localhost;
        proxy_set_header Cookie $http_cookie;
    }
    error_page 401 = @mpn_login;
    location @mpn_login { return 302 https://$host:` + fmt.Sprint(panelPort) + `/login; }

    # The program's own folders, and files that are not for a browser.
    location ~ ^/(libraries|templates|vendor|sql|locale|tmp|config|setup|doc|examples|test|node_modules)/ { return 404; }
    location ~ /\.(?!well-known) { return 404; }
    location ~* \.(inc|sh|md|lock|log|ini|dist|yml|yaml|xml|neon|twig|tpl|json|map)$ { return 404; }
    # Files that only say which version this is.
    location ~ ^/(README|ChangeLog|LICENSE|RELEASE-DATE.*|robots\.txt)$ { return 404; }

    location / {
        auth_request /_mpn_auth;
        try_files $uri $uri/ =404;
    }
    location ~ ^/(index|signon|logout|url|js/messages)\.php$ {
        auth_request /_mpn_auth;
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param HTTPS on;
        fastcgi_pass unix:/run/php/mpn-pma.sock;
        fastcgi_read_timeout 300s;
    }
    location ~ \.php$ { return 404; } # no other PHP file is run
}
`
}

const pmaCleanService = `[Unit]
Description=mpn drops the temporary users of the database browser that have expired

[Service]
Type=oneshot
ExecStart=` + mpnBinary + ` db pmaclean -apply
`

const pmaCleanTimer = `[Unit]
Description=mpn sweep of the temporary users of the database browser

[Timer]
OnCalendar=*:0/5
Persistent=true

[Install]
WantedBy=timers.target
`

// pmaStep installs the database browser. It needs the web panel, a database engine, and the Sury repository for the PHP
// it runs on.
func pmaStep(o Options) (Step, error) {
	if !o.Panel {
		return Step{}, fmt.Errorf("the database browser needs the web panel (-panel)")
	}
	if o.Engine == "" {
		return Step{}, fmt.Errorf("the database browser needs a database engine (-engine mariadb or mysql)")
	}
	if !slices.Contains(site.PHPVersions, pmaPHP) {
		return Step{}, fmt.Errorf("php %s is not offered", pmaPHP)
	}
	port := o.PanelPort
	if port == 0 {
		port = 2610
	}
	if PMAPort(port) > 65535 || PMAPort(port) == PanelBackendPort {
		return Step{}, fmt.Errorf("the database browser would need port %d: choose another panel port", PMAPort(port))
	}
	src := aptsrc.PHP(o.Target)
	acts := []Action{}
	if !slices.Contains(o.PHPVersions, pmaPHP) { // the PHP step has not installed it
		exts := []string{"fpm", "cli", "mysql", "mbstring", "xml", "curl", "zip", "gd"}
		pkgs := make([]string, len(exts))
		for i, e := range exts {
			pkgs[i] = "php" + pmaPHP + "-" + e
		}
		if len(o.PHPVersions) == 0 { // nothing has set the repository up
			acts = append(acts,
				Run("curl", "-fsSL", "--retry", "3", "-o", src.KeyFile, src.KeyURL),
				WriteFile(src.Path(), src.Content, 0o644),
				aptUpdate())
		}
		acts = append(acts, aptInstall(pkgs...))
	}
	backend := fmt.Sprint(PanelBackendPort)
	repl := strings.NewReplacer("@BACKEND@", backend, "@PANELPORT@", fmt.Sprint(port))
	acts = append(acts,
		Run("sh", "-c", pmaInstallScript),
		Run("sh", "-c", pmaStateScript),
		WriteFile(pmaDir+"/config.inc.php", pmaConfig(), 0o644),
		WriteFile(pmaDir+"/signon.php", repl.Replace(pmaSignon), 0o644),
		WriteFile(pmaDir+"/logout.php", repl.Replace(pmaLogout), 0o644),
		WriteFile("/etc/php/"+pmaPHP+"/fpm/pool.d/mpn-pma.conf", pmaPool(), 0o644),
		Run("php-fpm"+pmaPHP, "-t"),
		Run("systemctl", "enable", "php"+pmaPHP+"-fpm"),
		Run("systemctl", "restart", "php"+pmaPHP+"-fpm"),
		WriteFile("/etc/nginx/conf.d/02-mpn-pma.conf", pmaNginx(port), 0o644),
		Run("nginx", "-t"),
		ReloadNginx(),
		WriteFile("/etc/systemd/system/mpn-pma-clean.service", pmaCleanService, 0o644),
		WriteFile("/etc/systemd/system/mpn-pma-clean.timer", pmaCleanTimer, 0o644),
		Run("systemctl", "daemon-reload"),
		Run("systemctl", "enable", "--now", "mpn-pma-clean.timer"),
	)
	return Step{"database browser (phpMyAdmin " + pmaVersion + ")", acts}, nil
}
