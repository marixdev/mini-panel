// Package distro holds the paths, names and commands that differ between the Debian family (Debian, Ubuntu) and the RHEL family
// (AlmaLinux, Rocky Linux, CentOS Stream). Every other package asks this one instead of writing a path of either family itself.
//
// The family is read once from /etc/os-release when the program starts. A machine whose family cannot be told (a developer's
// laptop, a test) is treated as Debian, which is what the panel was written for first.
package distro

import (
	"os"
	"slices"
	"strings"
	"sync"
)

// Family is the family of the running system.
type Family int

const (
	Debian Family = iota // Debian and Ubuntu
	EL                   // AlmaLinux, Rocky Linux, CentOS Stream
)

var (
	once    sync.Once
	current Family
)

// Current returns the family of the running system.
func Current() Family {
	once.Do(func() {
		b, err := os.ReadFile("/etc/os-release")
		if err == nil {
			current = FromOSRelease(string(b))
		}
	})
	return current
}

// SetForTest fixes the family for the rest of the process. Tests use it to render the files of the other family.
func SetForTest(f Family) {
	once.Do(func() {})
	current = f
}

// FromOSRelease tells the family from the content of /etc/os-release.
func FromOSRelease(content string) Family {
	for _, line := range strings.Split(content, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || k != "ID" {
			continue
		}
		switch strings.Trim(v, `"'`) {
		case "almalinux", "rocky", "centos", "rhel":
			return EL
		}
	}
	return Debian
}

// IsEL reports whether the running system is of the RHEL family.
func IsEL() bool { return Current() == EL }

// ---- the web server ----------------------------------------------------------------------------------------------------------

// WebUser is the account nginx's workers run as, and the group that may read the sites' files.
func WebUser() string {
	if IsEL() {
		return "nginx"
	}
	return "www-data"
}

// LogGroup is the group that may read logs (adm on Debian; root on EL, which has no adm group for logs).
func LogGroup() string {
	if IsEL() {
		return "root"
	}
	return "adm"
}

// NginxModuleDir is where nginx's dynamic modules are.
func NginxModuleDir() string {
	if IsEL() {
		return "/usr/lib64/nginx/modules"
	}
	return "/usr/lib/nginx/modules"
}

// ---- PHP ------------------------------------------------------------------------------------------------------------------------
//
// Debian and Ubuntu (Sury): /etc/php/8.4/fpm/pool.d, unit php8.4-fpm, programs /usr/sbin/php-fpm8.4 and /usr/bin/php8.4.
// EL (Remi, PHP 7.4 and newer): /etc/opt/remi/php84/php-fpm.d, unit php84-php-fpm, programs /opt/remi/php84/root/usr/sbin/php-fpm
// and /opt/remi/php84/root/usr/bin/php. EL (built by mpn-packages, PHP 7.0 to 7.3): /opt/mpn/php/7.3/etc/php-fpm.d, unit
// mpn-php73-fpm, programs /opt/mpn/php/7.3/sbin/php-fpm and /opt/mpn/php/7.3/bin/php.

func short(v string) string { return strings.ReplaceAll(v, ".", "") }

// OwnPHP reports whether this PHP version comes from mpn-packages on EL rather than from Remi.
func OwnPHP(v string) bool { return IsEL() && v < "7.4" }

// PHPPoolDir is the folder of the pools of one PHP version, with a trailing slash.
func PHPPoolDir(v string) string {
	switch {
	case OwnPHP(v):
		return "/opt/mpn/php/" + v + "/etc/php-fpm.d/"
	case IsEL():
		return "/etc/opt/remi/php" + short(v) + "/php-fpm.d/"
	}
	return "/etc/php/" + v + "/fpm/pool.d/"
}

// PHPConfDir is the folder of the extra settings of the FPM of one version, with a trailing slash.
func PHPConfDir(v string) string {
	switch {
	case OwnPHP(v):
		return "/opt/mpn/php/" + v + "/etc/conf.d/"
	case IsEL():
		return "/etc/opt/remi/php" + short(v) + "/php.d/"
	}
	return "/etc/php/" + v + "/fpm/conf.d/"
}

// PHPDefaultPool is the pool the package installs, which mpn turns off so that only the sites' pools run.
func PHPDefaultPool(v string) string { return PHPPoolDir(v) + "www.conf" }

// PHPService is the systemd unit of the FPM of one version.
func PHPService(v string) string {
	switch {
	case OwnPHP(v):
		return "mpn-php" + short(v) + "-fpm"
	case IsEL():
		return "php" + short(v) + "-php-fpm"
	}
	return "php" + v + "-fpm"
}

// PHPFPMBinary is the FPM program of one version.
func PHPFPMBinary(v string) string {
	switch {
	case OwnPHP(v):
		return "/opt/mpn/php/" + v + "/sbin/php-fpm"
	case IsEL():
		return "/opt/remi/php" + short(v) + "/root/usr/sbin/php-fpm"
	}
	return "/usr/sbin/php-fpm" + v
}

// PHPCLIBinary is the command line program of one version.
func PHPCLIBinary(v string) string {
	switch {
	case OwnPHP(v):
		return "/opt/mpn/php/" + v + "/bin/php"
	case IsEL():
		return "/opt/remi/php" + short(v) + "/root/usr/bin/php"
	}
	return "/usr/bin/php" + v
}

// PHPPackages are the packages of one version with the given extensions ("fpm", "cli", "mysql", ...), in the names of the family.
// On EL, PHP 7.0 to 7.3 are not packages (see OwnPHP); the result is then empty.
func PHPPackages(v string, exts []string) []string {
	if OwnPHP(v) {
		return nil
	}
	var out []string
	for _, e := range exts {
		if !IsEL() {
			out = append(out, "php"+v+"-"+e)
			continue
		}
		// Remi names the server php-fpm and the client php-cli like Debian, but calls the MySQL driver php-mysqlnd, has curl inside
		// php-common and zip as php-pecl-zip (checked on AlmaLinux 9 with php84, 2026-11-13).
		switch e {
		case "mysql":
			e = "mysqlnd"
		case "curl":
			e = "common"
		case "zip":
			e = "pecl-zip"
		}
		p := "php" + short(v) + "-php-" + e
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// ---- databases ------------------------------------------------------------------------------------------------------------------

// DBConfFile is where mpn writes its tuning of the database server.
func DBConfFile(engine string) string {
	if IsEL() {
		return "/etc/my.cnf.d/99-mpn.cnf"
	}
	if engine == "mysql" {
		return "/etc/mysql/conf.d/99-mpn.cnf"
	}
	return "/etc/mysql/mariadb.conf.d/99-mpn.cnf"
}

// DBService is the systemd unit of the database server.
func DBService(engine string) string {
	if engine == "mysql" {
		if IsEL() {
			return "mysqld"
		}
		return "mysql"
	}
	return "mariadb"
}

// DBPackage is the server package of an engine.
func DBPackage(engine string) string {
	if engine == "mysql" {
		return "mysql-community-server"
	}
	if IsEL() {
		return "MariaDB-server"
	}
	return "mariadb-server"
}

// ---- other services -------------------------------------------------------------------------------------------------------------

// CronService is the unit that runs cron jobs, and CronPackage the package that brings it.
func CronService() string {
	if IsEL() {
		return "crond"
	}
	return "cron"
}

func CronPackage() string {
	if IsEL() {
		return "cronie"
	}
	return "cron"
}

// ---- packages -------------------------------------------------------------------------------------------------------------------

// InstallCommand is the command that installs packages without questions, waiting for another package manager to finish.
func InstallCommand(pkgs ...string) []string {
	if IsEL() {
		return append([]string{"dnf", "-y", "-q", "--setopt=install_weak_deps=False", "install"}, pkgs...)
	}
	return append([]string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "-o", "DPkg::Lock::Timeout=300", "install", "-y"}, pkgs...)
}

// RefreshScript reads the package lists again, retrying for about five minutes: a fresh server often runs its own updates first.
func RefreshScript() string {
	if IsEL() {
		return `for i in $(seq 1 30); do dnf -q makecache && exit 0; sleep 10; done
echo "dnf makecache kept failing for about 5 minutes" >&2; exit 1`
	}
	return `for i in $(seq 1 30); do apt-get -o DPkg::Lock::Timeout=300 update && exit 0; sleep 10; done
echo "apt-get update kept failing for about 5 minutes" >&2; exit 1`
}

// InstalledScript prints "yes" when the package is installed.
func InstalledScript(pkg string) string {
	if IsEL() {
		return "rpm -q " + pkg + " >/dev/null 2>&1 && echo yes || true"
	}
	return "dpkg-query -W -f='${Status}' " + pkg + " 2>/dev/null | grep -q 'install ok installed' && echo yes || true"
}

// RemoveCommand removes packages and their configuration.
func RemoveCommand(pkgs ...string) []string {
	if IsEL() {
		return append([]string{"dnf", "-y", "-q", "remove"}, pkgs...)
	}
	return append([]string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "-o", "DPkg::Lock::Timeout=300", "purge", "-y"}, pkgs...)
}
