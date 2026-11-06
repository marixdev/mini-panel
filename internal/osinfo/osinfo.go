// Package osinfo detects the supported Linux releases and where their packages come from.
package osinfo

import (
	"bufio"
	"fmt"
	"strings"
)

// Target is one supported OS release. Codename is the apt suite name.
type Target struct {
	ID       string // "debian" or "ubuntu"
	Version  string // "12", "22.04"
	Codename string
}

var supported = []Target{
	{"debian", "12", "bookworm"},
	{"debian", "13", "trixie"},
	{"ubuntu", "22.04", "jammy"},
	{"ubuntu", "24.04", "noble"},
	{"ubuntu", "26.04", "resolute"},
}

// Supported returns a copy of the supported release list.
func Supported() []Target { return append([]Target(nil), supported...) }

// Parse reads the content of /etc/os-release and returns the matching supported target.
func Parse(content string) (Target, error) {
	kv := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		if k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "="); ok {
			kv[k] = strings.Trim(v, `"'`)
		}
	}
	for _, t := range supported {
		if t.ID == kv["ID"] && t.Version == kv["VERSION_ID"] {
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("unsupported OS: %s %s", kv["ID"], kv["VERSION_ID"])
}

// Repository base URLs. Suites for all supported targets were checked on 2026-11-03:
// Sury PHP has every suite, MariaDB 11.8 has every suite, MySQL 8.4 LTS has the server package.
func (t Target) PHPRepo() string { return "https://packages.sury.org/php/" }
func (t Target) MariaDBRepo(series string) string {
	return "https://mirror.mariadb.org/repo/" + series + "/" + t.ID
}
func (t Target) MySQLRepo() string { return "https://repo.mysql.com/apt/" + t.ID }
