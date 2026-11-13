// Package osinfo detects the supported Linux releases and where their packages come from.
package osinfo

import (
	"bufio"
	"fmt"
	"strings"
)

// Target is one supported OS release. Codename is the apt suite name on Debian and Ubuntu, and "el9" or "el10" on the RHEL family
// (AlmaLinux, Rocky Linux and CentOS Stream of one major version share their packages, so they share the files built for them).
type Target struct {
	ID       string // "debian", "ubuntu", "almalinux", "rocky" or "centos"
	Version  string // "12", "22.04", "9", "10"
	Codename string
}

var supported = []Target{
	{"debian", "12", "bookworm"},
	{"debian", "13", "trixie"},
	{"ubuntu", "22.04", "jammy"},
	{"ubuntu", "24.04", "noble"},
	{"ubuntu", "26.04", "resolute"},
	{"almalinux", "9", "el9"},
	{"almalinux", "10", "el10"},
	{"rocky", "9", "el9"},
	{"rocky", "10", "el10"},
	{"centos", "9", "el9"},
	{"centos", "10", "el10"},
}

// Supported returns a copy of the supported release list.
func Supported() []Target { return append([]Target(nil), supported...) }

// Parse reads the content of /etc/os-release and returns the matching supported target. On the RHEL family only the major
// version counts: AlmaLinux 9.8 and Rocky Linux 9.6 are both "9".
func Parse(content string) (Target, error) {
	kv := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "="); ok {
			kv[k] = strings.Trim(v, `"'`)
		}
	}
	id, ver := kv["ID"], kv["VERSION_ID"]
	if isEL(id) {
		ver, _, _ = strings.Cut(ver, ".")
	}
	for _, t := range supported {
		if t.ID == id && t.Version == ver {
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("unsupported OS: %s %s", kv["ID"], kv["VERSION_ID"])
}

func isEL(id string) bool { return id == "almalinux" || id == "rocky" || id == "centos" }

// EL reports whether the system is of the RHEL family, whose packages come from dnf and whose layout differs from Debian's.
func (t Target) EL() bool { return isEL(t.ID) }

// NginxOrg reports whether this system gets its nginx from nginx.org at one fixed version. Every supported system does except
// Ubuntu 26.04, whose own nginx is that same version and whose modules are packaged for it. A zero Target (a system the program
// does not know) keeps the nginx of the distribution.
func (t Target) NginxOrg() bool {
	switch t.ID {
	case "debian":
		return t.Codename != ""
	case "ubuntu":
		return t.Codename != "" && t.Version != "26.04"
	}
	return t.EL()
}

// Repository base URLs. Suites for all supported targets were checked on 2026-11-03:
// Sury PHP has every suite, MariaDB 11.8 has every suite, MySQL 8.4 LTS has the server package.
func (t Target) PHPRepo() string { return "https://packages.sury.org/php/" }
func (t Target) MariaDBRepo(series string) string {
	return "https://mirror.mariadb.org/repo/" + series + "/" + t.ID
}
func (t Target) MySQLRepo() string { return "https://repo.mysql.com/apt/" + t.ID }
