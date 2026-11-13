package osinfo

import "slices"

// Database series offered on each release. The lists follow what CloudPanel's install.sh installs on the same
// releases, limited to what the vendors' own apt repositories publish for the release (checked 2026-11-03):
//   - MariaDB 10.6 exists only for Ubuntu 22.04; 10.11 and 11.4 are not published for Debian 13 or Ubuntu 26.04.
//   - The Oracle repository has no MySQL 8.0 for Debian 13 or Ubuntu 26.04. CloudPanel gets its "MySQL 8.0" from
//     Percona Server through its own repository, which mpn does not use.
//   - MySQL 9.x is not offered: it removed the mysql_native_password plugin that PHP older than 7.4 needs.
//   - MySQL 8.4 on Ubuntu 22.04 is not in CloudPanel's list; it is offered because it was installed and run here.
var (
	mariadbSeries = map[string][]string{
		"debian-12":    {"10.11", "11.4", "11.8", "12.3"},
		"debian-13":    {"11.8", "12.3"},
		"ubuntu-22.04": {"10.6", "10.11", "11.4", "11.8"},
		"ubuntu-24.04": {"10.11", "11.4", "11.8", "12.3"},
		"ubuntu-26.04": {"11.8", "12.3"},
	}
	mysqlSeries = map[string][]string{
		"debian-12":    {"8.0", "8.4"},
		"debian-13":    {"8.4"},
		"ubuntu-22.04": {"8.0", "8.4"},
		"ubuntu-24.04": {"8.0", "8.4"},
		"ubuntu-26.04": {"8.4"},
	}
)

// On the RHEL family the series come from rpm.mariadb.org and repo.mysql.com (checked 2026-11-13 for both CPUs). AlmaLinux, Rocky
// Linux and CentOS Stream share the key, because they share the repositories. repo.mysql.com has no MySQL 8.0 for el10.
func init() {
	for _, el := range []string{"el9", "el10"} {
		mariadbSeries[el] = []string{"10.11", "11.4", "11.8"}
	}
	mysqlSeries["el9"] = []string{"8.0", "8.4"}
	mysqlSeries["el10"] = []string{"8.4"}
}

func (t Target) key() string {
	if t.EL() {
		return t.Codename
	}
	return t.ID + "-" + t.Version
}

// Series returns the series offered for an engine ("mariadb" or "mysql") on this release, oldest first.
func (t Target) Series(engine string) []string {
	switch engine {
	case "mariadb":
		return slices.Clone(mariadbSeries[t.key()])
	case "mysql":
		return slices.Clone(mysqlSeries[t.key()])
	}
	return nil
}

func (t Target) HasSeries(engine, series string) bool {
	return slices.Contains(t.Series(engine), series)
}

// DefaultSeries is used when no series is asked for. Both defaults are offered on every supported release.
func DefaultSeries(engine string) string {
	switch engine {
	case "mariadb":
		return "11.8"
	case "mysql":
		return "8.4"
	}
	return ""
}
