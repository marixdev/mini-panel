package installer

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"mpn/internal/distro"
	"mpn/internal/osinfo"
	"mpn/internal/site"
	"mpn/internal/tune"
)

// The RHEL family (AlmaLinux, Rocky Linux, CentOS Stream 9 and 10). The same programs come from other places there:
//   - nginx 1.30.7 from nginx.org, as on Debian, in /etc/yum.repos.d/nginx.repo and locked with versionlock;
//   - PHP 7.4 and newer from Remi, which needs EPEL; PHP 7.0 to 7.3 and the ModSecurity module from mpn-packages;
//   - MariaDB from rpm.mariadb.org and MySQL from repo.mysql.com, whose keys are checked by fingerprint like nginx.org's;
//   - SELinux stays on: the panel gives the web roots, the site logs and its own ports the labels nginx and PHP-FPM need.

const (
	elPackagesRelease = "el-r3"
	elPackagesBase    = "https://github.com/marixdev/mpn-packages/releases/download/" + elPackagesRelease

	// Fingerprints of the signing keys of the RPM repositories, read on 2026-11-13. A key that does not hold its fingerprint is
	// refused, and dnf then checks every package of the repository against the key.
	mariadbSigningFPR = "177F4010FE56CA3336300305F1656F24C74CD1D8"
	mysqlSigningFPR   = "BCA43417C3B485DD128EC6D4B7B3B788A8D3785C"
)

// elSums are the SHA-256 of the files of the el-r3 release of mpn-packages, by file name.
var elSums = map[string]string{
	"ngx_http_modsecurity_module-el9-nginx1.30.7.so":        "4eae1556c15ca81787056d50ebd8c06638b7f3e4ba4fcd6c7bf776ed29429c44",
	"ngx_http_modsecurity_module-el9-nginx1.30.7-arm64.so":  "ad52dcea7098a6585d1439fe383bbb49bcbafeaf13681c9c24717f4911c2f67d",
	"ngx_http_modsecurity_module-el10-nginx1.30.7.so":       "97e39b6c024d4a2ba57ed2b2db95522083b6320aa08adf61259aab1ea6e26c16",
	"ngx_http_modsecurity_module-el10-nginx1.30.7-arm64.so": "209cb5434f37dfc3569d478af65c82136304868fa2f2b97584fe4bd527ddae53",
	"coreruleset-4.25.2.tar.gz":                             "a3ecf5d5b0f8001e2eaf2006448fc8e22d6b645d7fc1c71268be5b99a7395a2d",
	"php-5.6.40-el9.tar.xz":                                 "fad05691e092fde35b837052582ce02e7cd8dbeac5dd933b2518d19e1fc56809",
	"php-5.6.40-el9-arm64.tar.xz":                           "40c8dad86b8bc2f6c807e9f4def8d2eb6ecc77e34c653e13ab4d0ec501ebcc53",
	"php-5.6.40-el10.tar.xz":                                "e93d8e801af83074618a742afc81c1a56df491b219babb1586b213daf4740f3f",
	"php-5.6.40-el10-arm64.tar.xz":                          "c97e0f2dc16be948e70be9e219cf7a08794e3e6f321bfae452168b6568205ce6",
	"php-7.0.33-el9.tar.xz":                                 "67758c036f55726a25f85a5e83f4a2cc22b85e32639ad5020274f4dafa093bb0",
	"php-7.0.33-el9-arm64.tar.xz":                           "b822a183e4658d2d078f88b8fe5eb283187679c3c0101f0ec4ff774fb3beb86f",
	"php-7.0.33-el10.tar.xz":                                "eda5ed755f466ea41f01dd4ef918d5f00171d4c27b51a033c56091a907d8c1bb",
	"php-7.0.33-el10-arm64.tar.xz":                          "40c758575b33cd4c506593dbbc9674d7a9f334f9709eb2f740c601a8ce792af9",
	"php-7.1.33-el9.tar.xz":                                 "f111336f21c1ceb45b8fbcd9e65141cb275289ca3191ac7c38d1b8f079ec76b7",
	"php-7.1.33-el9-arm64.tar.xz":                           "5bef02b87d7b574eb06e6270278733dfcfcac6c7cbace7de699d221141db4b15",
	"php-7.1.33-el10.tar.xz":                                "dd26a7973876bdc492ddffbc979f00f5bc1a534e7f1bd66b5067bff01a641a11",
	"php-7.1.33-el10-arm64.tar.xz":                          "af3192a725825ea93b88d345d9bb58c08fca2cc5a30063b6893a8d0c9e7b6c03",
	"php-7.2.34-el9.tar.xz":                                 "6d7be53a3241fbdfbd8494f932f14305a8ea89a79c3f33f6d5561ad16136948a",
	"php-7.2.34-el9-arm64.tar.xz":                           "65db9a5e1ae4dbb27977a47c9603f8b440b38f4257425622acd4092b573b07c7",
	"php-7.2.34-el10.tar.xz":                                "2cd703267652095ca80358a4727114cb12ade0e86399a0848b401c0b702d5341",
	"php-7.2.34-el10-arm64.tar.xz":                          "856ac55e4975e66bf169c590d76b1b7eb52c36815d5abc090faad1bb5c634039",
	"php-7.3.33-el9.tar.xz":                                 "a2d3658671695a1221d8bd6e761ce6ffd386598126ff06afdd387b6a138a1b5a",
	"php-7.3.33-el9-arm64.tar.xz":                           "84d5706c8723bc4049996857095c6ca9bd85cce0dfc90d1104da04a0600e45c4",
	"php-7.3.33-el10.tar.xz":                                "8f0b6ff87a1c715a5c4c7701c87d07890e838986f9216010c5b62f79436c6472",
	"php-7.3.33-el10-arm64.tar.xz":                          "d7417a2cf44eebac281a6bd76d4029fde72b1f49fff654964699812dfa6ad870",
}

// ownPHPRelease is the release of each PHP version that mpn-packages builds for EL.
var ownPHPRelease = map[string]string{"5.6": "5.6.40", "7.0": "7.0.33", "7.1": "7.1.33", "7.2": "7.2.34", "7.3": "7.3.33"}

// CRSVersion is the OWASP Core Rule Set that the panel installs on EL, where no package has it.
const CRSVersion = "4.25.2"

func archSuffix() string {
	if runtime.GOARCH == "arm64" {
		return "-arm64"
	}
	return ""
}

// elFetch downloads one file of the el-r3 release into dest after its checksum was compared with the one in this program.
func elFetch(name, dest string) (Action, error) {
	sum := elSums[name]
	if sum == "" {
		return Action{}, fmt.Errorf("there is no checksum for %s in this program", name)
	}
	return Run("sh", "-c", fetchScript(elPackagesBase+"/"+name, sum, dest)), nil
}

// ---- repositories --------------------------------------------------------------------------------------------------------------

// keyScript fetches a signing key over HTTPS, keeps it only when it holds the fingerprint, and imports it into rpm.
func keyScript(url, fpr, dest string) string {
	return `set -eu
t=$(mktemp); trap 'rm -f "$t"' EXIT
curl -fsSL --retry 3 --proto '=https' '` + url + `' -o "$t"
gpg --batch --with-colons --show-keys "$t" | awk -F: '$1=="fpr"{print $10}' | grep -qx ` + fpr + ` \
  || { echo "the key from ` + url + ` is not the one this installer expects" >&2; exit 1; }
install -m 0644 "$t" ` + dest + `
rpm --import ` + dest
}

// elNginxRepo is the nginx.org repository. module_hotfixes lets it win over the nginx module stream of EL9.
const elNginxRepo = `# Written by mpn.
[nginx-stable]
name=nginx stable repo
baseurl=https://nginx.org/packages/centos/$releasever/$basearch/
gpgcheck=1
enabled=1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-nginx
module_hotfixes=true
`

// elNginxInstallActions installs nginx 1.30.7 from nginx.org and locks it at that version.
func elNginxInstallActions() []Action {
	return []Action{
		Run(distro.InstallCommand("gnupg2", "dnf-plugins-core", "python3-dnf-plugin-versionlock")...),
		Run("sh", "-c", keyScript("https://nginx.org/keys/nginx_signing.key", nginxSigningFPR, "/etc/pki/rpm-gpg/RPM-GPG-KEY-nginx")),
		WriteFile("/etc/yum.repos.d/nginx.repo", elNginxRepo, 0o644),
		Run(distro.InstallCommand("nginx-" + NginxVersion)...),
		// versionlock keeps "dnf upgrade" from moving nginx, so the modules built for this version keep loading.
		Run("sh", "-c", "dnf versionlock list 2>/dev/null | grep -q '^nginx-' || dnf -q versionlock add nginx"),
		Run("sh", "-c", `v=$(nginx -v 2>&1); case "$v" in *nginx/`+NginxVersion+`*) ;; *) echo "nginx `+NginxVersion+` was expected, this machine has: $v" >&2; exit 1;; esac`),
		Run("install", "-d", "-m", "0755", nginxModulesEnabled, "/etc/nginx/sites-enabled"),
		Run("rm", "-f", "/etc/nginx/conf.d/default.conf"),
	}
}

// elEPEL installs EPEL (Remi needs it, and so do fail2ban and OpenDKIM) and turns on CRB, where some of its dependencies are.
func elEPEL() Action {
	return Run("sh", "-c", `set -e
rpm -q epel-release >/dev/null 2>&1 || dnf -y -q install epel-release
dnf config-manager --set-enabled crb 2>/dev/null || dnf config-manager --set-enabled powertools 2>/dev/null || true`)
}

// elRemi installs the release package of Remi from the address Remi publishes. dnf then checks every package of Remi against the
// keys that package brings.
func elRemi(t osinfo.Target) Action {
	return Run("sh", "-c", `set -e
rpm -q remi-release >/dev/null 2>&1 || dnf -y -q install https://rpms.remirepo.net/enterprise/remi-release-`+t.Version+`.rpm`)
}

// elMariaDBRepo is the repository of one MariaDB series at rpm.mariadb.org.
func elMariaDBRepo(series string) string {
	return `# Written by mpn.
[mariadb]
name=MariaDB ` + series + `
baseurl=https://rpm.mariadb.org/` + series + `/rhel/$releasever/$basearch
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-MariaDB
gpgcheck=1
module_hotfixes=1
`
}

// elMySQLRepo is the repository of one MySQL series at repo.mysql.com.
func elMySQLRepo(series string) string {
	return `# Written by mpn.
[mysql-community]
name=MySQL ` + series + ` Community Server
baseurl=https://repo.mysql.com/yum/mysql-` + series + `-community/el/$releasever/$basearch/
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-mysql
gpgcheck=1
module_hotfixes=1
`
}

// elDatabaseActions add the repository of the engine and install its server. The databases of the system (the mysql and mariadb
// module streams of EL9) are left out by module_hotfixes and by turning the modules off.
func elDatabaseActions(engine, series string) []Action {
	var acts []Action
	switch engine {
	case "mariadb":
		acts = []Action{
			Run("sh", "-c", keyScript("https://rpm.mariadb.org/RPM-GPG-KEY-MariaDB", mariadbSigningFPR, "/etc/pki/rpm-gpg/RPM-GPG-KEY-MariaDB")),
			WriteFile("/etc/yum.repos.d/mariadb.repo", elMariaDBRepo(series), 0o644),
		}
	case "mysql":
		acts = []Action{
			Run("sh", "-c", keyScript("https://repo.mysql.com/RPM-GPG-KEY-mysql-2025", mysqlSigningFPR, "/etc/pki/rpm-gpg/RPM-GPG-KEY-mysql")),
			WriteFile("/etc/yum.repos.d/mysql-community.repo", elMySQLRepo(series), 0o644),
		}
	}
	return append(acts,
		Run("sh", "-c", "dnf -y -q module disable mysql mariadb 2>/dev/null || true"),
		Run(distro.InstallCommand(distro.DBPackage(engine))...),
		Run("install", "-d", "-m", "0755", "/etc/my.cnf.d"),
	)
}

// ---- PHP -----------------------------------------------------------------------------------------------------------------------

// elPHPActions install one PHP version: from Remi for 7.4 and newer, from mpn-packages for 7.0 to 7.3. A version from mpn-packages
// gets its own systemd unit (mpn-php73-fpm), since there is no package to bring one.
func elPHPActions(t osinfo.Target, v string, exts []string) ([]Action, error) {
	if !distro.OwnPHP(v) {
		return []Action{Run(distro.InstallCommand(distro.PHPPackages(v, exts)...)...)}, nil
	}
	rel, ok := ownPHPRelease[v]
	if !ok {
		return nil, fmt.Errorf("php %s is not built for %s", v, t.Codename)
	}
	name := "php-" + rel + "-" + t.Codename + archSuffix() + ".tar.xz"
	fetch, err := elFetch(name, "/var/cache/mpn/"+name)
	if err != nil {
		return nil, err
	}
	// The libraries the build links against, which a minimal system may lack.
	// libcurl is asked for as libcurl.so.4 (the file, not a package): the images ship libcurl-minimal, which provides it, and asking for
	// the package libcurl conflicts with that one (seen in Rocky Linux 9 and CentOS Stream 9 and 10 containers).
	libs := []string{"libxml2", "libcurl.so.4()(64bit)", "sqlite-libs", "oniguruma", "libzip", "bzip2-libs", "libpng", "libjpeg-turbo", "libwebp", "freetype", "xz"}
	if t.Codename == "el9" {
		libs = append(libs, "libicu")
	}
	dir := "/opt/mpn/php/" + v
	return []Action{
		Run(distro.InstallCommand(libs...)...),
		fetch,
		Run("sh", "-c", `set -e
f=/var/cache/mpn/`+name+`
if [ ! -x `+dir+`/sbin/php-fpm ] || [ "$(cat `+dir+`/.mpn-archive 2>/dev/null)" != "$(sha256sum "$f" | cut -d' ' -f1)" ]; then
  tar -C / -xJf "$f"
  sha256sum "$f" | cut -d' ' -f1 > `+dir+`/.mpn-archive
fi
ln -sfn `+dir+`/bin/php /usr/local/bin/php`+v),
		WriteFile("/etc/systemd/system/"+distro.PHPService(v)+".service", ownPHPUnit(v), 0o644),
		Run("systemctl", "daemon-reload"),
		// The label the SELinux step gave the path; the program arrived after that step. An FPM that was started before its program had
		// the label keeps the domain it started in (a reload does not change it), and nginx may not connect to its sockets; it is
		// restarted when restorecon changed a label.
		Run("sh", "-c", `command -v restorecon >/dev/null || exit 0
if [ -n "$(restorecon -R -v `+dir+`)" ] && systemctl is-active --quiet `+distro.PHPService(v)+`; then systemctl restart `+distro.PHPService(v)+`; fi`),
	}, nil
}

// ownPHPUnit runs the FPM of a PHP version built by mpn-packages, as the packages of Remi run theirs.
func ownPHPUnit(v string) string {
	dir := "/opt/mpn/php/" + v
	return `# Written by mpn.
[Unit]
Description=PHP ` + v + ` FastCGI Process Manager (mpn)
After=network.target

[Service]
Type=simple
ExecStartPre=` + dir + `/sbin/php-fpm --test --fpm-config ` + dir + `/etc/php-fpm.conf
ExecStart=` + dir + `/sbin/php-fpm --nodaemonize --fpm-config ` + dir + `/etc/php-fpm.conf
ExecReload=/bin/kill -USR2 $MAINPID
PrivateTmp=true
RuntimeDirectory=php-fpm-` + short(v) + `
RuntimeDirectoryPreserve=yes

[Install]
WantedBy=multi-user.target
`
}

func short(v string) string { return strings.ReplaceAll(v, ".", "") }

// RunPHPDir makes /run/php, where every site's pool puts its socket on both families (provision.PHPSocket). Debian's packages make
// it themselves; on EL a tmpfiles.d entry makes it at every boot, with the SELinux label of a web server's runtime files.
func RunPHPDir() []Action {
	if !distro.IsEL() {
		return nil
	}
	return []Action{
		WriteFile("/etc/tmpfiles.d/mpn-php.conf", "# Written by mpn.\nd /run/php 0755 root root -\n", 0o644),
		Run("systemd-tmpfiles", "--create", "/etc/tmpfiles.d/mpn-php.conf"),
		Run("sh", "-c", "command -v restorecon >/dev/null && restorecon -R /run/php || true"),
	}
}

// ---- SELinux -------------------------------------------------------------------------------------------------------------------

// selinuxScript gives what the panel serves the labels nginx and PHP-FPM may use, and allows them the network: nginx proxies to the
// panel, to node and python sites; PHP reaches the database over TCP and other sites over HTTPS (found on AlmaLinux 9, 2026-11-13,
// with nginx.org's nginx and Remi's PHP: without these, every page of a site in /home was 403 and every proxy 502). Nothing is done
// when SELinux is off. ports are the TCP ports nginx listens on besides 80 and 443.
func selinuxScript(ports []int) string {
	var p strings.Builder
	for _, n := range ports {
		s := strconv.Itoa(n)
		p.WriteString("semanage port -a -t http_port_t -p tcp " + s + " 2>/dev/null || semanage port -m -t http_port_t -p tcp " + s + "\n")
	}
	return `set -e
command -v getenforce >/dev/null || exit 0
[ "$(getenforce)" != Disabled ] || exit 0
command -v semanage >/dev/null || dnf -y -q install policycoreutils-python-utils
label() { semanage fcontext -a -t "$1" "$2" 2>/dev/null || semanage fcontext -m -t "$1" "$2"; }
# The web root of every site: nginx reads it, PHP-FPM writes uploads and caches into it.
label httpd_sys_rw_content_t '/home/[^/]+/htdocs(/.*)?'
label httpd_log_t '/var/log/mpn/sites(/.*)?'
label httpd_sys_content_t '/usr/share/mpn(/.*)?'
label httpd_sys_rw_content_t '/var/lib/mpn-pma(/.*)?'
label httpd_cache_t '/var/cache/ngx_pagespeed(/.*)?'
# The FPM of PHP 7.0 to 7.3 from mpn-packages must run in the domain of a web server, as Remi's does (its program is labelled
# httpd_exec_t); otherwise its sockets get a label nginx may not connect to (502, seen on AlmaLinux 9).
label httpd_exec_t '/opt/mpn/php/[^/]+/sbin/php-fpm'
# The sockets of the sites' pools (provision.PHPSocket). The policy's default for /run/php is var_run_t, which nginx may not
# connect to: a restorecon of the folder then gave every socket that label and every PHP page was 502 (seen on AlmaLinux 9).
# semanage takes only one spelling, because an equivalency rule maps the other to it: /var/run on EL9, /run on EL10.
label httpd_var_run_t '/run/php(/.*)?' 2>/dev/null || label httpd_var_run_t '/var/run/php(/.*)?'
[ ! -d /run/php ] || restorecon -R /run/php
# The socket of the panel (mpn-web.socket), which nginx and the database browser connect to. systemd makes it with the label of the
# path, so the rule must exist before the socket is started.
label httpd_var_run_t '/run/mpn-web(/.*)?' 2>/dev/null || label httpd_var_run_t '/var/run/mpn-web(/.*)?'
[ ! -d /run/mpn-web ] || restorecon -R /run/mpn-web
` + panelSELinuxModule + `# systemd may start a program only from a path labelled as one: the interpreter of a python site's virtual environment is in the
# home of the site, user_home_t ("Unable to locate executable .../venv/bin/python: Permission denied", seen on Rocky Linux 10).
label bin_t '/home/[^/]+/venv/bin(/.*)?'
label bin_t '/opt/node/[^/]+/bin(/.*)?'
[ ! -d /opt/node ] || restorecon -R /opt/node
[ ! -d /opt/mpn/php ] || restorecon -R /opt/mpn/php
install -d -m 0755 /var/log/mpn/sites
restorecon -R /var/log/mpn /home 2>/dev/null || true
[ ! -d /usr/share/mpn ] || restorecon -R /usr/share/mpn
[ ! -d /var/cache/ngx_pagespeed ] || restorecon -R /var/cache/ngx_pagespeed
setsebool -P httpd_can_network_connect=1 httpd_can_sendmail=1
` + p.String()
}

// SELinuxStep is the SELinux step of the plan on EL. ports are the panel's own ports (panel, database browser, web mail).
func SELinuxStep(ports []int) Step {
	return Step{"selinux", []Action{Run("sh", "-c", selinuxScript(ports))}}
}

// RestoreconHome labels a web root that was just made, so nginx and PHP-FPM may use it. It does nothing where SELinux is off.
func RestoreconHome(path string) Action {
	return Run("sh", "-c", "command -v restorecon >/dev/null && [ \"$(getenforce 2>/dev/null)\" != Disabled ] && restorecon -R '"+path+"' || true")
}

// ---- steps of the plan ---------------------------------------------------------------------------------------------------------

// elPHPStep installs each PHP version on EL and leaves its FPM stopped until a site uses it, as on Debian.
func elPHPStep(o Options) (Step, error) {
	acts := []Action{elEPEL()}
	if slices.ContainsFunc(o.PHPVersions, func(v string) bool { return !distro.OwnPHP(v) }) {
		acts = append(acts, elRemi(o.Target))
	}
	acts = append(acts, RunPHPDir()...)
	for _, v := range o.PHPVersions {
		if !slices.Contains(site.PHPVersions, v) {
			return Step{}, fmt.Errorf("php version %q is not offered", v)
		}
		exts := []string{"fpm", "cli", "mysql", "mbstring", "xml", "curl", "zip", "gd", "opcache"}
		pa, err := elPHPActions(o.Target, v, exts)
		if err != nil {
			return Step{}, err
		}
		acts = append(acts, pa...)
		acts = append(acts,
			WriteFile(distro.PHPConfDir(v)+"99-mpn.ini", o.Profile.OpcacheINIFor(v), 0o644),
			Run("sh", "-c", stopUnusedFPM(v)),
		)
	}
	return Step{"php " + strings.Join(o.PHPVersions, ","), acts}, nil
}

// elDatabaseStep installs the database server from its vendor's repository and writes the tuned config.
func elDatabaseStep(o Options, series string) (Step, error) {
	if o.Engine != "mariadb" && o.Engine != "mysql" {
		return Step{}, fmt.Errorf("unknown engine %q (want mariadb or mysql)", o.Engine)
	}
	if !o.Target.HasSeries(o.Engine, series) {
		return Step{}, fmt.Errorf("%s %s is not offered on %s %s; choose one of: %s", o.Engine, series, o.Target.ID, o.Target.Version, strings.Join(o.Target.Series(o.Engine), ", "))
	}
	cnf, err := o.Profile.DBConf(o.Engine)
	if err != nil {
		return Step{}, err
	}
	if o.Engine == "mysql" && hasPHPBefore74(o.PHPVersions) {
		cnf += tune.LegacyAuthConf(series)
	}
	svc := distro.DBService(o.Engine)
	acts := append(elDatabaseActions(o.Engine, series),
		WriteFile(distro.DBConfFile(o.Engine), cnf, 0o644),
		Run("systemctl", "enable", svc),
		Run("systemctl", "restart", svc),
	)
	return Step{"database " + o.Engine, acts}, nil
}

// elPanelPorts are the ports nginx listens on for the panel: the panel itself, the database browser after it, and the web mail.
func elPanelPorts(o Options) []int {
	p := o.PanelPort
	if p == 0 {
		p = 2610
	}
	// The web mail is the second port after the panel's (mail.WebmailPort); 2612 was written here before, which was wrong for
	// any panel port but 2610.
	return []int{p, pmaPortFor(o, p), p + 2}
}

// ---- base packages -------------------------------------------------------------------------------------------------------------

// elBasePackages are the base packages on EL. curl-minimal, which the images ship, already gives curl; asking for curl would
// conflict with it on EL9.
func elBasePackages() []string {
	return []string{"ca-certificates", "tar", "xz", "openssl", "logrotate", distro.CronPackage(), "policycoreutils-python-utils", "shadow-utils", "procps-ng", "gnupg2"}
}

// elFirewallAction turns firewalld off: the panel's own firewall is nftables, as on Debian, and two firewalls would each block what
// the other opens.
func elFirewallAction() Action {
	return Run("sh", "-c", "if systemctl is-active --quiet firewalld; then systemctl disable --now firewalld; echo 'firewalld turned off: the panel manages the firewall with nftables'; fi")
}

// elWAFActions install the ModSecurity module for nginx.org's nginx on EL (libmodsecurity is inside the module) and the OWASP Core
// Rule Set, in the places the Debian package uses so the rest of the panel finds them.
func elWAFActions(t osinfo.Target) ([]Action, error) {
	mod := "ngx_http_modsecurity_module-" + t.Codename + "-nginx" + NginxVersion + archSuffix() + ".so"
	fetchMod, err := elFetch(mod, distro.NginxModuleDir()+"/ngx_http_modsecurity_module.so")
	if err != nil {
		return nil, err
	}
	crs := "coreruleset-" + CRSVersion + ".tar.gz"
	fetchCRS, err := elFetch(crs, "/var/cache/mpn/"+crs)
	if err != nil {
		return nil, err
	}
	n := moduleNames[ModModSecurity]
	return []Action{
		// libmodsecurity is linked into the module; these are the system libraries it still loads.
		Run(distro.InstallCommand("pcre2", "libxml2", "libcurl.so.4()(64bit)")...),
		fetchMod,
		Run("install", "-d", "-m", "0755", nginxModulesEnabled),
		WriteFile(nginxModulesEnabled+"/"+n.snippet, "# Written by mpn.\nload_module "+distro.NginxModuleDir()+"/"+n.so+";\n", 0o644),
		fetchCRS,
		Run("sh", "-c", `set -e
f=/var/cache/mpn/`+crs+`
d=/usr/share/modsecurity-crs
if [ "$(cat $d/.mpn-version 2>/dev/null)" != `+CRSVersion+` ]; then
  rm -rf $d.new; mkdir -p $d.new
  tar -xzf "$f" -C $d.new --strip-components=1
  rm -rf $d; mv $d.new $d
  echo `+CRSVersion+` > $d/.mpn-version
fi
install -d -m 0755 /etc/modsecurity/crs
[ -f /etc/modsecurity/crs/crs-setup.conf ] || cp $d/crs-setup.conf.example /etc/modsecurity/crs/crs-setup.conf
for x in REQUEST-900-EXCLUSION-RULES-BEFORE-CRS RESPONSE-999-EXCLUSION-RULES-AFTER-CRS; do
  [ -f /etc/modsecurity/crs/$x.conf ] || cp $d/rules/$x.conf.example /etc/modsecurity/crs/$x.conf
done
command -v restorecon >/dev/null && restorecon -R $d /etc/modsecurity || true`),
		WriteFile("/etc/nginx/modsecurity.conf", ModsecBaseline, 0o644),
	}, nil
}
