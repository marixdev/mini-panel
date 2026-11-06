// Package store keeps hosted sites in a SQLite database.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, so the binary stays static

	"mpn/internal/site"
)

var (
	ErrExists   = errors.New("already exists")
	ErrNotFound = errors.New("not found")
	ErrInUse    = errors.New("still in use")
)

// migrations[i] upgrades a database from version i to i+1. Never edit an entry a release has already used.
var migrations = []string{
	schemaV1,
	`ALTER TABLE sites ADD COLUMN tls INTEGER NOT NULL DEFAULT 0`,
	`CREATE TABLE databases (
    name       TEXT PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE RESTRICT,
    created_at INTEGER NOT NULL
);
CREATE INDEX databases_site ON databases(site_id);`,
	// Settings a person may change later: the PHP settings (JSON) and the memory of a node app.
	`ALTER TABLE sites ADD COLUMN php_ini TEXT;
ALTER TABLE sites ADD COLUMN mem_mb INTEGER NOT NULL DEFAULT 0;`,
	// Python sites. SQLite cannot change a CHECK constraint, so the table is rebuilt. The marker on the first line makes
	// runMigration switch foreign keys off for it: dropping the old table with them on would delete every hostname row
	// through ON DELETE CASCADE.
	noForeignKeys + `
CREATE TABLE sites_new (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    kind         TEXT NOT NULL CHECK (kind IN ('static','php','node','proxy','python')),
    system_user  TEXT NOT NULL UNIQUE,
    root_dir     TEXT NOT NULL,
    php_version  TEXT,
    app_port     INTEGER UNIQUE,
    node_version TEXT,
    entry        TEXT,
    upstream     TEXT,
    created_at   INTEGER NOT NULL,
    tls          INTEGER NOT NULL DEFAULT 0,
    php_ini      TEXT,
    mem_mb       INTEGER NOT NULL DEFAULT 0,
    py_version   TEXT
);
INSERT INTO sites_new (id, name, kind, system_user, root_dir, php_version, app_port, node_version, entry, upstream, created_at, tls, php_ini, mem_mb)
    SELECT id, name, kind, system_user, root_dir, php_version, app_port, node_version, entry, upstream, created_at, tls, php_ini, mem_mb FROM sites;
DROP TABLE sites;
ALTER TABLE sites_new RENAME TO sites;`,
	// The domain choices of a site: the www redirect and whether plain HTTP is still served once HTTPS is on.
	`ALTER TABLE sites ADD COLUMN www TEXT NOT NULL DEFAULT '';
ALTER TABLE sites ADD COLUMN allow_http INTEGER NOT NULL DEFAULT 0;`,
	// Where the certificate comes from. Every site that has SSL until now got it from Let's Encrypt.
	`ALTER TABLE sites ADD COLUMN ssl_mode TEXT NOT NULL DEFAULT '';
UPDATE sites SET ssl_mode = 'letsencrypt' WHERE tls = 1;`,
	// The nginx configuration a person wrote by hand; empty means the one mpn makes.
	`ALTER TABLE sites ADD COLUMN vhost TEXT NOT NULL DEFAULT '';`,
}

// noForeignKeys, at the start of a migration, runs it with foreign key enforcement off and checks them again afterwards.
const noForeignKeys = "-- mpn:no-foreign-keys"

var schemaVersion = len(migrations)

const schemaV1 = `
CREATE TABLE sites (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    kind        TEXT NOT NULL CHECK (kind IN ('static','php','node','proxy')),
    system_user TEXT NOT NULL UNIQUE,
    root_dir    TEXT NOT NULL,
    php_version TEXT,
    app_port    INTEGER UNIQUE,
    node_version TEXT,
    entry       TEXT,
    upstream    TEXT,
    created_at  INTEGER NOT NULL
);
CREATE TABLE hostnames (
    hostname TEXT PRIMARY KEY,
    site_id  INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    position INTEGER NOT NULL
);
CREATE INDEX hostnames_site ON hostnames(site_id);
`

type Store struct{ db *sql.DB }

// Open opens (and if needed creates) the database at path and brings its schema up to date.
func Open(path string) (*Store, error) {
	// The database is for root alone. Creating it with mode 0600 first leaves no moment when another account could read it.
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600); err == nil {
		f.Close()
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer, and it keeps the foreign_keys pragma on every statement
	s := &Store{db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	restrict(path)
	return s, nil
}

// restrict makes the database and its journal files private. SQLite gives the journal files the mode of the main file when
// it creates them, but files left by an earlier version keep the mode they had.
func restrict(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Chmod(path+suffix, 0o600)
	}
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	switch {
	case v == schemaVersion:
		return nil
	case v > schemaVersion:
		return fmt.Errorf("database schema %d is newer than this build supports (%d)", v, schemaVersion)
	}
	for i := v; i < len(migrations); i++ {
		if err := s.runMigration(i); err != nil {
			return fmt.Errorf("migrating database to version %d: %w", i+1, err)
		}
	}
	return nil
}

func (s *Store) runMigration(i int) error {
	if strings.HasPrefix(migrations[i], noForeignKeys) {
		// The pragma has no effect inside a transaction. There is one connection, so it holds for the whole migration.
		if _, err := s.db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
			return err
		}
		defer s.db.Exec("PRAGMA foreign_keys = ON")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(migrations[i]); err != nil {
		return err
	}
	if strings.HasPrefix(migrations[i], noForeignKeys) {
		var table string
		if err := tx.QueryRow("PRAGMA foreign_key_check").Scan(&table); err == nil {
			return fmt.Errorf("the rebuilt tables break a foreign key in %q", table)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
		return err
	}
	return tx.Commit()
}

func nullStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// existsError says what was already taken, in words a user can act on, and matches ErrExists with errors.Is.
type existsError struct{ what string }

func (e existsError) Error() string        { return e.what + " is already taken" }
func (e existsError) Is(target error) bool { return target == ErrExists }

// uniqueLabels names the columns whose UNIQUE constraint a user can run into.
var uniqueLabels = map[string]string{
	"sites.name": "site name", "sites.system_user": "system user", "sites.app_port": "node port",
	"hostnames.hostname": "hostname", "databases.name": "database name",
}

// wrap turns SQLite's "UNIQUE constraint failed: hostnames.hostname (1555)" into "hostname is already taken".
func wrap(err error) error {
	msg := err.Error()
	const marker = "UNIQUE constraint failed: "
	i := strings.Index(msg, marker)
	if i < 0 {
		return err
	}
	col := strings.TrimSpace(strings.SplitN(msg[i+len(marker):], " (", 2)[0])
	if label, ok := uniqueLabels[col]; ok {
		return existsError{label}
	}
	return existsError{col}
}

// AddSite validates and stores a site with its hostnames, all or nothing.
// Fields that do not belong to the site's kind are not stored.
func (s *Store) AddSite(st site.Site) error {
	if err := st.Validate(); err != nil {
		return err
	}
	// The default system user equals the site name, so a repeated name would otherwise be reported as a taken
	// "system user". The UNIQUE columns still guard against a race between two callers.
	var one int
	if s.db.QueryRow(`SELECT 1 FROM sites WHERE name = ?`, st.Name).Scan(&one) == nil {
		return existsError{"site name"}
	}
	var php, upstream, phpIni any
	var port, nodeVer, entry, pyVer any
	mem := 0
	switch st.Kind {
	case site.PHP:
		php = nullStr(st.PHPVersion)
		phpIni = iniJSON(st.PHPIni)
	case site.Node:
		port, nodeVer, entry, mem = st.NodePort, nullStr(st.NodeVersion), nullStr(st.Entry), st.MemMB
	case site.Python:
		port, pyVer, entry, mem = st.NodePort, nullStr(st.PyVersion), nullStr(st.Entry), st.MemMB
	case site.Proxy:
		upstream = nullStr(st.Upstream)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	mode := sslModeOf(st)
	res, err := tx.Exec(`INSERT INTO sites (name, kind, system_user, root_dir, php_version, app_port, node_version, py_version, entry, upstream, tls, ssl_mode, php_ini, mem_mb, www, allow_http, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, st.Name, string(st.Kind), st.User, st.Root, php, port, nodeVer, pyVer, entry, upstream, mode != "", mode, phpIni, mem, st.WWW, st.AllowHTTP, time.Now().Unix())
	if err != nil {
		return wrap(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for i, h := range st.Hostnames {
		if _, err := tx.Exec(`INSERT INTO hostnames (hostname, site_id, position) VALUES (?,?,?)`, h, id, i); err != nil {
			return wrap(err)
		}
	}
	return tx.Commit()
}

func (s *Store) GetSite(name string) (site.Site, error) {
	list, err := s.load("WHERE name = ?", name)
	if err != nil {
		return site.Site{}, err
	}
	if len(list) == 0 {
		return site.Site{}, fmt.Errorf("site %q: %w", name, ErrNotFound)
	}
	return list[0], nil
}

func (s *Store) ListSites() ([]site.Site, error) { return s.load("") }

// sslModeOf is the certificate source of a site. A site that has SSL but no recorded source got it from Let's Encrypt, as
// every site did before there was a choice.
func sslModeOf(st site.Site) string {
	if st.SSLMode == site.SSLNone && st.TLS {
		return site.SSLLetsEncrypt
	}
	if !st.TLS {
		return site.SSLNone
	}
	return st.SSLMode
}

// SetSSL records where the certificate of a site comes from, or that it has none. The site is served over SSL exactly
// when it has a source.
func (s *Store) SetSSL(name, mode string) error {
	if !site.ValidSSLMode(mode) {
		return fmt.Errorf("unknown SSL source %q", mode)
	}
	res, err := s.db.Exec(`UPDATE sites SET tls = ?, ssl_mode = ? WHERE name = ?`, mode != "", mode, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("site %q: %w", name, ErrNotFound)
	}
	return nil
}

// SetVhost stores the nginx configuration a person wrote for a site, or "" to go back to the one mpn makes.
func (s *Store) SetVhost(name, text string) error {
	res, err := s.db.Exec(`UPDATE sites SET vhost = ? WHERE name = ?`, text, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("site %q: %w", name, ErrNotFound)
	}
	return nil
}

// SetTLS records whether a site is served over SSL, from Let's Encrypt when it is.
func (s *Store) SetTLS(name string, on bool) error {
	if on {
		return s.SetSSL(name, site.SSLLetsEncrypt)
	}
	return s.SetSSL(name, site.SSLNone)
}

// SetDomain stores the domain of a site: its host names (the address first), the www choice, whether plain HTTP is still
// served and whether HTTPS is on. The host names are replaced all together, so a failure leaves the old ones.
func (s *Store) SetDomain(st site.Site) error {
	if err := st.Validate(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`SELECT id FROM sites WHERE name = ?`, st.Name).Scan(&id); err != nil {
		return fmt.Errorf("site %q: %w", st.Name, ErrNotFound)
	}
	mode := sslModeOf(st)
	if _, err := tx.Exec(`UPDATE sites SET www = ?, allow_http = ?, tls = ?, ssl_mode = ? WHERE id = ?`, st.WWW, st.AllowHTTP, mode != "", mode, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM hostnames WHERE site_id = ?`, id); err != nil {
		return err
	}
	for i, h := range st.Hostnames {
		if _, err := tx.Exec(`INSERT INTO hostnames (hostname, site_id, position) VALUES (?,?,?)`, h, id, i); err != nil {
			return wrap(err)
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteSite(name string) error {
	res, err := s.db.Exec(`DELETE FROM sites WHERE name = ?`, name)
	if err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			return fmt.Errorf("site %q still has databases: %w", name, ErrInUse)
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("site %q: %w", name, ErrNotFound)
	}
	return nil
}

// AllocNodePort returns the lowest port in [lo, hi] that no site uses. Two callers can get the same
// port; the UNIQUE column then makes the second AddSite fail with ErrExists.
func (s *Store) AllocNodePort(lo, hi int) (int, error) {
	rows, err := s.db.Query(`SELECT app_port FROM sites WHERE app_port BETWEEN ? AND ? ORDER BY app_port`, lo, hi)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	next := lo
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return 0, err
		}
		if p > next {
			break
		}
		next = p + 1
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if next > hi {
		return 0, fmt.Errorf("no free port between %d and %d", lo, hi)
	}
	return next, nil
}

// load reads sites matching where (a fixed string from this package, never user input) and their hostnames.
func (s *Store) load(where string, args ...any) ([]site.Site, error) {
	rows, err := s.db.Query(`SELECT id, name, kind, system_user, root_dir, php_version, app_port, node_version, py_version, entry, upstream, tls, ssl_mode, php_ini, mem_mb, www, allow_http, vhost
		FROM sites `+where+` ORDER BY name`, args...)
	if err != nil {
		return nil, err
	}
	var out []site.Site
	index := map[int64]int{}
	for rows.Next() {
		var id int64
		var st site.Site
		var kind string
		var php, up, nodeVer, pyVer, entry, ini sql.NullString
		var port sql.NullInt64
		if err := rows.Scan(&id, &st.Name, &kind, &st.User, &st.Root, &php, &port, &nodeVer, &pyVer, &entry, &up, &st.TLS, &st.SSLMode, &ini, &st.MemMB, &st.WWW, &st.AllowHTTP, &st.CustomVhost); err != nil {
			rows.Close()
			return nil, err
		}
		st.SSLMode = sslModeOf(st)
		if ini.String != "" {
			if err := json.Unmarshal([]byte(ini.String), &st.PHPIni); err != nil {
				rows.Close()
				return nil, fmt.Errorf("site %q: the stored PHP settings are not readable: %w", st.Name, err)
			}
		}
		st.Kind, st.PHPVersion, st.NodePort, st.Upstream = site.Kind(kind), php.String, int(port.Int64), up.String
		st.NodeVersion, st.PyVersion, st.Entry = nodeVer.String, pyVer.String, entry.String
		index[id] = len(out)
		out = append(out, st)
	}
	err = rows.Err()
	rows.Close() // must be closed before the next query because the pool has one connection
	if err != nil {
		return nil, err
	}
	hrows, err := s.db.Query(`SELECT site_id, hostname FROM hostnames
		WHERE site_id IN (SELECT id FROM sites `+where+`) ORDER BY site_id, position`, args...)
	if err != nil {
		return nil, err
	}
	defer hrows.Close()
	for hrows.Next() {
		var id int64
		var h string
		if err := hrows.Scan(&id, &h); err != nil {
			return nil, err
		}
		out[index[id]].Hostnames = append(out[index[id]].Hostnames, h)
	}
	return out, hrows.Err()
}

// iniJSON stores the PHP settings of a site, or NULL when there are none.
func iniJSON(m map[string]string) any {
	if len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m) // a map of strings always marshals
	return string(b)
}

// UpdateSite stores the settings of a site that a person may change: its web root, PHP version and settings, the version,
// port, start script and memory of a node app, and the address a proxy forwards to. The name, kind and account never
// change; asking for that is an error, so a caller cannot rename a site into somebody else's account by mistake.
func (s *Store) UpdateSite(st site.Site) error {
	if err := st.Validate(); err != nil {
		return err
	}
	old, err := s.GetSite(st.Name)
	if err != nil {
		return err
	}
	if old.Kind != st.Kind || old.User != st.User {
		return fmt.Errorf("site %q: the kind and the account of a site cannot be changed", st.Name)
	}
	var php, upstream, phpIni, port, nodeVer, pyVer, entry any
	mem := 0
	switch st.Kind {
	case site.PHP:
		php, phpIni = nullStr(st.PHPVersion), iniJSON(st.PHPIni)
	case site.Node:
		port, nodeVer, entry, mem = st.NodePort, nullStr(st.NodeVersion), nullStr(st.Entry), st.MemMB
	case site.Python:
		port, pyVer, entry, mem = st.NodePort, nullStr(st.PyVersion), nullStr(st.Entry), st.MemMB
	case site.Proxy:
		upstream = nullStr(st.Upstream)
	}
	res, err := s.db.Exec(`UPDATE sites SET root_dir = ?, php_version = ?, php_ini = ?, app_port = ?, node_version = ?, py_version = ?, entry = ?, upstream = ?, mem_mb = ?
		WHERE name = ?`, st.Root, php, phpIni, port, nodeVer, pyVer, entry, upstream, mem, st.Name)
	if err != nil {
		return wrap(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("site %q: %w", st.Name, ErrNotFound)
	}
	return nil
}
