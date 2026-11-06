// Package aptsrc builds apt repository definitions (deb822 format) for PHP, MariaDB and MySQL.
package aptsrc

import (
	"fmt"
	"strings"

	"mpn/internal/osinfo"
)

// Source is one third-party apt repository together with the key that signs it.
type Source struct {
	Name    string // file name without extension
	KeyURL  string
	KeyFile string // where the downloaded key is stored
	Content string // deb822 text for Path()
}

func (s Source) Path() string { return "/etc/apt/sources.list.d/" + s.Name + ".sources" }

func build(name, keyURL, keyFile, uri, suite string, components ...string) Source {
	c := fmt.Sprintf("Types: deb\nURIs: %s\nSuites: %s\nComponents: %s\nSigned-By: %s\n",
		uri, suite, strings.Join(components, " "), keyFile)
	return Source{name, keyURL, keyFile, c}
}

// PHP returns the Sury repository. Its key is a binary keyring, so the file keeps the .gpg extension.
func PHP(t osinfo.Target) Source {
	return build("sury-php", "https://packages.sury.org/php/apt.gpg", "/usr/share/keyrings/sury-php.gpg",
		t.PHPRepo(), t.Codename, "main")
}

// MariaDB returns the repository for one series such as "11.8" (ASCII-armored key).
func MariaDB(t osinfo.Target, series string) (Source, error) {
	if err := checkSeries(t, "mariadb", series); err != nil {
		return Source{}, err
	}
	return build("mariadb", "https://mariadb.org/mariadb_release_signing_key.asc", "/usr/share/keyrings/mariadb.asc",
		t.MariaDBRepo(series), t.Codename, "main"), nil
}

// checkSeries fails with the list of series that are offered, so the message tells the user what to pick instead.
func checkSeries(t osinfo.Target, engine, series string) error {
	if t.HasSeries(engine, series) {
		return nil
	}
	name := map[string]string{"mariadb": "MariaDB", "mysql": "MySQL"}[engine]
	return fmt.Errorf("%s %s is not offered on %s %s; choose one of: %s",
		name, series, t.ID, t.Version, strings.Join(t.Series(engine), ", "))
}

// mysqlComponent maps a series to its component in the Oracle repository.
var mysqlComponent = map[string]string{"8.4": "mysql-8.4-lts", "8.0": "mysql-8.0"}

// MySQL returns the Oracle MySQL repository for one series such as "8.4". The 2025 key is an assumption:
// the repo also serves a 2023 key and which one signs Release has not been checked yet.
func MySQL(t osinfo.Target, series string) (Source, error) {
	if err := checkSeries(t, "mysql", series); err != nil {
		return Source{}, err
	}
	return build("mysql", "https://repo.mysql.com/RPM-GPG-KEY-mysql-2025", "/usr/share/keyrings/mysql.asc",
		t.MySQLRepo(), t.Codename, mysqlComponent[series]), nil
}
