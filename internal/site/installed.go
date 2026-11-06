package site

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Which versions are really on this machine. The lists in site.go are what mpn knows how to install; a form must only
// offer what is installed, or choosing a version would fail at the last step with a message about a missing program.

// InstalledPHP returns the PHP versions whose FPM program exists in dir (/usr/sbin), oldest first.
func InstalledPHP(dir string) []string {
	var out []string
	for _, v := range PHPVersions {
		if fi, err := os.Stat(filepath.Join(dir, "php-fpm"+v)); err == nil && !fi.IsDir() {
			out = append(out, v)
		}
	}
	return out
}

// InstalledPython returns the Python 3 minor versions that have an interpreter in dir (/usr/bin), oldest first. A version
// counts only when its interpreter is a program that is there; python3 itself (a link to one of them) is not listed apart.
func InstalledPython(dir string) []string {
	var out []string
	for minor := 6; minor <= 39; minor++ {
		v := "3." + strconv.Itoa(minor)
		if fi, err := os.Stat(filepath.Join(dir, "python"+v)); err == nil && !fi.IsDir() {
			out = append(out, v)
		}
	}
	return out
}

// SystemPython returns the minor version that /usr/bin/python3 points at, or "" when it cannot be told.
func SystemPython() string {
	target, err := filepath.EvalSymlinks("/usr/bin/python3")
	if err != nil {
		return ""
	}
	v := strings.TrimPrefix(filepath.Base(target), "python")
	if ValidPythonVersion(v) {
		return v
	}
	return ""
}

// InstalledNode returns the Node.js major versions installed under root (/opt/node), oldest first.
func InstalledNode(root string) []string {
	var out []string
	for _, v := range NodeVersions {
		if fi, err := os.Stat(filepath.Join(root, v, "bin", "node")); err == nil && !fi.IsDir() {
			out = append(out, v)
		}
	}
	return out
}
