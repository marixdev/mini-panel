// Package tune derives service settings (database, PHP-FPM, nginx, swap) from the host's RAM and CPU count.
//
// Every number here is a starting point chosen for low-RAM hosts. None of it has been measured
// on a real workload yet; phpWorkerMB in particular must be replaced by a measured value.
package tune

import (
	"fmt"
	"strings"
)

// phpWorkerMB is an assumed average size of one PHP-FPM worker.
const phpWorkerMB = 40

type Input struct{ RAMMB, CPUs int }

type Profile struct {
	Tier         string
	SwapMB       int
	JournalMaxMB int

	BufferPoolMB   int
	RedoMB         int
	MaxConnections int
	TmpTableMB     int
	PerfSchema     bool

	PHPMaxChildren   int
	PHPMemoryLimitMB int
	OpcacheMB        int

	NginxWorkers      int
	WorkerConnections int
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

// New builds a profile. RAM below 512MB is treated as 512MB and CPUs below 1 as 1.
func New(in Input) Profile {
	ram, cpus := max(in.RAMMB, 512), max(in.CPUs, 1)
	p := Profile{
		BufferPoolMB:     clamp(ram/8, 64, 4096),
		MaxConnections:   clamp(ram/25, 30, 300),
		TmpTableMB:       clamp(ram/64, 16, 128),
		PerfSchema:       ram > 2048,
		PHPMaxChildren:   clamp(ram*35/100/phpWorkerMB, 2, 64),
		PHPMemoryLimitMB: 256,
		OpcacheMB:        clamp(ram/16, 64, 256),
		NginxWorkers:     cpus,
	}
	p.RedoMB = clamp(p.BufferPoolMB/4, 48, 1024)
	switch {
	case ram <= 1024:
		p.Tier, p.SwapMB, p.JournalMaxMB = "small", 1024, 50
		p.NginxWorkers, p.WorkerConnections = min(cpus, 2), 1024
	case ram <= 2048:
		p.Tier, p.SwapMB, p.JournalMaxMB, p.WorkerConnections = "medium", 1024, 100, 2048
	case ram <= 4096:
		p.Tier, p.SwapMB, p.JournalMaxMB, p.WorkerConnections = "large", 2048, 100, 2048
		p.PHPMemoryLimitMB = 512
	default:
		p.Tier, p.SwapMB, p.JournalMaxMB, p.WorkerConnections = "xlarge", 2048, 100, 2048
		p.PHPMemoryLimitMB = 512
	}
	return p
}

// DBConf renders a [mysqld] snippet. engine is "mariadb" or "mysql" (they name the redo log setting differently).
func (p Profile) DBConf(engine string) (string, error) {
	var b strings.Builder
	b.WriteString("[mysqld]\n")
	// Sites reach the database over loopback; nothing outside the server should. Packages from the vendors'
	// own repositories (unlike Debian's) listen on every address by default.
	b.WriteString("bind-address = 127.0.0.1\n")
	fmt.Fprintf(&b, "innodb_buffer_pool_size = %dM\n", p.BufferPoolMB)
	switch engine {
	case "mariadb":
		fmt.Fprintf(&b, "innodb_log_file_size = %dM\n", p.RedoMB)
	case "mysql":
		fmt.Fprintf(&b, "innodb_redo_log_capacity = %dM\n", p.RedoMB)
		b.WriteString("mysqlx-bind-address = 127.0.0.1\n") // MariaDB does not know this option and would not start
	default:
		return "", fmt.Errorf("unknown engine %q (want mariadb or mysql)", engine)
	}
	perf := "OFF"
	if p.PerfSchema {
		perf = "ON"
	}
	fmt.Fprintf(&b, "max_connections = %d\n", p.MaxConnections)
	fmt.Fprintf(&b, "tmp_table_size = %dM\n", p.TmpTableMB)
	fmt.Fprintf(&b, "max_heap_table_size = %dM\n", p.TmpTableMB)
	fmt.Fprintf(&b, "performance_schema = %s\n", perf)
	return b.String(), nil
}

// LegacyAuthConf returns the MySQL settings appended when PHP older than 7.4 is installed, for one series. Those
// clients cannot read the "caching_sha2_password" default that MySQL 8 announces in its handshake, so the older plugin
// is made the default. The older plugin is weaker, which is why it is not on by default.
//   - 8.4 (tested with PHP 7.0 on 8.4.13): the plugin is off unless enabled, and enabling it alone is not enough;
//     authentication_policy has to name it as well.
//   - 8.0: the plugin is always loaded and the "mysql_native_password" option does not exist there (it would stop the
//     server). The handshake plugin comes from default_authentication_plugin; setting only authentication_policy
//     leaves PHP 7.0 unable to log in (tested on 8.0.46).
//
// Other series (MariaDB, or any MySQL 9) get nothing.
func LegacyAuthConf(series string) string {
	switch series {
	case "8.4":
		return "mysql_native_password = ON\nauthentication_policy = mysql_native_password\n"
	case "8.0":
		return "default_authentication_plugin = mysql_native_password\n"
	}
	return ""
}

// PHPPool renders one per-site pool with the machine's own memory limit. The socket is owned by www-data, which nginx runs
// as on Debian and Ubuntu.
func (p Profile) PHPPool(name, user, socket string) string {
	return p.PHPPoolWith(name, user, socket, []string{fmt.Sprintf("php_admin_value[memory_limit] = %dM", p.PHPMemoryLimitMB)}, 300)
}

// PHPPoolWith renders a pool whose PHP settings are the given lines (see site.PHPPoolLines), and whose requests end after
// terminateSeconds. The lines are made by the site package from a fixed list of settings and values it has validated.
func (p Profile) PHPPoolWith(name, user, socket string, settings []string, terminateSeconds int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `[%s]
user = %s
group = %s
listen = %s
listen.owner = www-data
listen.group = www-data
pm = ondemand
pm.max_children = %d
pm.process_idle_timeout = 10s
pm.max_requests = 500
request_terminate_timeout = %ds
`, name, user, user, socket, p.PHPMaxChildren, terminateSeconds)
	for _, l := range settings {
		b.WriteString(l + "\n")
	}
	return b.String()
}

// OpcacheINI renders the shared opcache settings. They are per FPM master, so they cannot live in a pool.
func (p Profile) OpcacheINI() string {
	return fmt.Sprintf("opcache.enable=1\nopcache.memory_consumption=%d\nopcache.jit=off\nopcache.validate_timestamps=1\nopcache.revalidate_freq=2\n", p.OpcacheMB)
}
