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
//   - nginx 1.28.3 from nginx.org, as on Debian, in /etc/yum.repos.d/nginx.repo and locked with versionlock;
//   - PHP 7.4 and newer from Remi, which needs EPEL; PHP 7.0 to 7.3 and the ModSecurity module from mpn-packages;
//   - MariaDB from rpm.mariadb.org and MySQL from repo.mysql.com, whose keys are checked by fingerprint like nginx.org's;
//   - SELinux stays on: the panel gives the web roots, the site logs and its own ports the labels nginx and PHP-FPM need.

const (
	elPackagesRelease = "el-r1"
	elPackagesBase    = "https://github.com/marixdev/mpn-packages/releases/download/" + elPackagesRelease

	// Fingerprints of the signing keys of the RPM repositories, read on 2026-11-13. A key that does not hold its fingerprint is
	// refused, and dnf then checks every package of the repository against the key.
	mariadbSigningFPR = "177F4010FE56CA3336300305F1656F24C74CD1D8"
	mysqlSigningFPR   = "BCA43417C3B485DD128EC6D4B7B3B788A8D3785C"
)

// elSums are the SHA-256 of the files of the el-r1 release of mpn-packages, by file name.
var elSums = map[string]string{
	"ngx_http_modsecurity_module-el9-nginx1.28.3.so":        "afb853945d3b0856daff12b893068fa44396b23ba04cfa15dd25343ac5e0ee01",
	"ngx_http_modsecurity_module-el9-nginx1.28.3-arm64.so":  "462d8a428554f226e4e29812bb1109ce7f39cf0a561a00c988e2ca9383ea24b8",
	"ngx_http_modsecurity_module-el10-nginx1.28.3.so":       "a22aef4301c678000de2a18be84a514602764878ca5c6044c0bc0108b62afa0f",
	"ngx_http_modsecurity_module-el10-nginx1.28.3-arm64.so": "8598c8ccadbb47bb531efaa0ac0d89f41ac4e795e5175bd7577b1662161f8ae6",
	"coreruleset-4.25.2.tar.gz":                             "a3ecf5d5b0f8001e2eaf2006448fc8e22d6b645d7fc1c71268be5b99a7395a2d",
	"php-7.0.33-el9.tar.xz":                                 "2a31f19c2a1916415962e542006dc68dca1e51ac02e5283ffc08a86bb0687c35",
	"php-7.0.33-el9-arm64.tar.xz":                           "0eeed4947cad56a088f5ba1be8394f50c566a4ec389a0a8020e1e9be474130a3",
	"php-7.0.33-el10.tar.xz":                                "2a675962167bc0b9b280afe6ff918ec69318f0385bf38731b38b3886fcfebf26",
	"php-7.0.33-el10-arm64.tar.xz":                          "5118fe0ef30a10dc2f2e4a2f72f8af50282bc6eacf4e1d35b61b26901ba050bb",
	"php-7.1.33-el9.tar.xz":                                 "e6757087812d5fa60cc0f1f11dad9b23d03ecd57fc7fe6f6c983f313f410781c",
	"php-7.1.33-el9-arm64.tar.xz":                           "d121b6da83d23fa8c782255ea616d1fd207c4cd916cf5adfab7c809b6b27a186",
	"php-7.1.33-el10.tar.xz":                                "1ef73754077280b950a876ccc9fd052f74fc81130e4d2a3c990dadb76174f2f5",
	"php-7.1.33-el10-arm64.tar.xz":                          "ae34a4c641c4a4cf1d142f9faffed263b0df8f3117b0917ce1b80a07e53df3af",
	"php-7.2.34-el9.tar.xz":                                 "7ed5862c3f2026777ca690e67a016a48f7cb90a244042b36ac0f55dbecff91f1",
	"php-7.2.34-el9-arm64.tar.xz":                           "111e5815a413aed5ec965491d9420b8291119cdcf1f33e92a516638d8da7bd63",
	"php-7.2.34-el10.tar.xz":                                "9544894a1b01f1e4be81c6bd5e5385a5b4dcb47ebccec0a24562e67012914a0e",
	"php-7.2.34-el10-arm64.tar.xz":                          "cbc4dfc5a71cea84e90c4f0b70ae0e9219454eb6f7f863baeb402fd43f75ef63",
	"php-7.3.33-el9.tar.xz":                                 "bc2f5d224c0b7fa0d26c9df7b157b8a354c0c3b703dd191047e8f645670a155f",
	"php-7.3.33-el9-arm64.tar.xz":                           "214f067487f543a9fb043edc171219366d7d7730e740caaa0b2de27ceff3e9e3",
	"php-7.3.33-el10.tar.xz":                                "e63b96567c0d42fb972fb53c9c0536e03865893cc46826cf828b1c2f8982c2b9",
	"php-7.3.33-el10-arm64.tar.xz":                          "df7ccf64649c975dae2ff0bf008ad08301f100c0012cc814afd77f0de21a93a9",
}

// ownPHPRelease is the release of each PHP version that mpn-packages builds for EL.
var ownPHPRelease = map[string]string{"7.0": "7.0.33", "7.1": "7.1.33", "7.2": "7.2.34", "7.3": "7.3.33"}

// CRSVersion is the OWASP Core Rule Set that the panel installs on EL, where no package has it.
const CRSVersion = "4.25.2"

func archSuffix() string {
	if runtime.GOARCH == "arm64" {
		return "-arm64"
	}
	return ""
}

// elFetch downloads one file of the el-r1 release into dest after its checksum was compared with the one in this program.
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

// elNginxInstallActions installs nginx 1.28.3 from nginx.org and locks it at that version.
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
	libs := []string{"libxml2", "libcurl", "sqlite-libs", "oniguruma", "libzip", "bzip2-libs", "libpng", "libjpeg-turbo", "libwebp", "freetype", "xz"}
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
			WriteFile(distro.PHPConfDir(v)+"99-mpn.ini", o.Profile.OpcacheINI(), 0o644),
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
	return []int{p, PMAPort(p), 2612}
}

// ---- base packages -------------------------------------------------------------------------------------------------------------

// elBasePackages are the base packages on EL. curl-minimal, which the images ship, already gives curl; asking for curl would
// conflict with it on EL9.
func elBasePackages() []string {
	return []string{"ca-certificates", "tar", "xz", "openssl", "logrotate", distro.CronPackage(), "policycoreutils-python-utils", "shadow-utils", "procps-ng"}
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
		Run(distro.InstallCommand("pcre2", "libxml2", "libcurl")...),
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
