// Package backup builds the plans that save a site (its web root and its databases) and put it back. Like provision,
// it only describes the work; nothing here touches the machine.
//
// A backup is a directory /var/backups/mpn/<site>/<stamp>/ owned by root with mode 0700, holding files.tar.gz, one
// <database>.sql.gz per database, and a MANIFEST written last. A directory without MANIFEST is an unfinished backup.
package backup

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"mpn/internal/dbadmin"
	"mpn/internal/installer"
	"mpn/internal/site"
)

// Base is the directory that holds every site's backups. Only root can enter it.
const Base = "/var/backups/mpn"

var stampRe = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}$`)

// ValidStamp reports whether s looks like 20261104-031500, the name of one backup.
func ValidStamp(s string) bool { return stampRe.MatchString(s) }

func SiteDir(name string) string    { return Base + "/" + name }
func Dir(name, stamp string) string { return SiteDir(name) + "/" + stamp }

// Manifest says what a backup holds.
type Manifest struct {
	Site  string
	Stamp string
	Files bool
	DBs   []string
}

const header = "mpn backup 1"

func (m Manifest) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nsite=%s\nstamp=%s\n", header, m.Site, m.Stamp)
	if m.Files {
		b.WriteString("files=files.tar.gz\n")
	}
	for _, d := range m.DBs {
		fmt.Fprintf(&b, "db=%s\n", d)
	}
	return b.String()
}

// ParseManifest reads a MANIFEST. Every value is checked, because a restore puts the names into shell commands.
func ParseManifest(text string) (Manifest, error) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if lines[0] != header {
		return Manifest{}, fmt.Errorf("not an mpn backup manifest (first line %q)", lines[0])
	}
	var m Manifest
	for _, l := range lines[1:] {
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			return Manifest{}, fmt.Errorf("manifest line %q has no '='", l)
		}
		switch k {
		case "site":
			if !site.ValidName(v) {
				return Manifest{}, fmt.Errorf("manifest has an invalid site name %q", v)
			}
			m.Site = v
		case "stamp":
			if !ValidStamp(v) {
				return Manifest{}, fmt.Errorf("manifest has an invalid stamp %q", v)
			}
			m.Stamp = v
		case "files":
			if v != "files.tar.gz" {
				return Manifest{}, fmt.Errorf("manifest has an unexpected files entry %q", v)
			}
			m.Files = true
		case "db":
			if !dbadmin.ValidName(v) {
				return Manifest{}, fmt.Errorf("manifest has an invalid database name %q", v)
			}
			m.DBs = append(m.DBs, v)
		default:
			return Manifest{}, fmt.Errorf("manifest has an unknown key %q", k)
		}
	}
	if m.Site == "" || m.Stamp == "" {
		return Manifest{}, fmt.Errorf("manifest lacks a site or a stamp")
	}
	return m, nil
}

// checkDBs makes sure every database name is safe and belongs to the site.
func checkDBs(siteName string, dbs []string) error {
	for _, d := range dbs {
		if !dbadmin.ValidName(d) || !dbadmin.OwnedBy(d, siteName) {
			return fmt.Errorf("database %q is not a database of site %q", d, siteName)
		}
	}
	return nil
}

// CreatePlan saves the web root and the given databases. dump is the path of mariadb-dump or mysqldump.
//
// The archive is made by the site's own user, not by root: root reading a directory the user controls could be tricked
// with a symlink into copying a file the user cannot read, and the user could then get it back with a restore.
// tar's exit status 1 means only that a file changed while it was read, which is normal on a live site. An empty or
// cut-off archive (for instance when runuser itself fails, which also exits 1) is caught by gzip -t.
func CreatePlan(s site.Site, stamp string, dbs []string, dump string) (installer.Plan, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if !ValidStamp(stamp) {
		return nil, fmt.Errorf("invalid backup stamp %q", stamp)
	}
	if len(dbs) > 0 && !site.SafePath(dump) {
		return nil, fmt.Errorf("invalid dump program %q", dump)
	}
	if err := checkDBs(s.Name, dbs); err != nil {
		return nil, err
	}
	dir := Dir(s.Name, stamp)
	files := dir + "/files.tar.gz"
	plan := installer.Plan{
		{Name: "backup directory", Actions: []installer.Action{
			installer.Run("install", "-d", "-m", "0700", "-o", "root", "-g", "root", Base, SiteDir(s.Name)),
			installer.Run("mkdir", "-m", "0700", dir), // fails when it exists, so a backup is never overwritten
		}},
		{Name: "files", Actions: []installer.Action{installer.Run("sh", "-c", fmt.Sprintf(
			`[ -d %[2]s ] || { echo 'site root %[2]s does not exist' >&2; exit 1; }; umask 077; runuser -u %[1]s -- tar -C %[2]s -czf - . > %[3]s; rc=$?; [ "$rc" -le 1 ] && gzip -t %[3]s || { echo 'archiving the site files failed' >&2; exit 1; }`,
			s.User, s.Root, files))}},
	}
	for _, d := range dbs {
		sql := fmt.Sprintf("%s/%s.sql", dir, d)
		plan = append(plan, installer.Step{Name: "database " + d, Actions: []installer.Action{installer.Run("sh", "-c", fmt.Sprintf(
			`umask 077; %[1]s --single-transaction --routines --events --triggers --default-character-set=utf8mb4 %[2]s > %[3]s || { rm -f %[3]s; echo 'dump of %[2]s failed' >&2; exit 1; }; gzip -f %[3]s`,
			dump, d, sql))}})
	}
	m := Manifest{Site: s.Name, Stamp: stamp, Files: true, DBs: dbs}
	return append(plan, installer.Step{Name: "manifest", Actions: []installer.Action{
		installer.WriteFile(dir+"/MANIFEST", m.String(), 0o600),
	}}), nil
}

// RestorePlan puts back the files and/or the given databases from one backup.
//
// Files are extracted by the site's user, so the archive can only write where that user could write anyway. Files
// that exist now but are not in the backup stay. Each dump drops and re-creates its own tables; the database itself
// must already exist, and tables created since the backup stay. client is the path of the mariadb or mysql program.
func RestorePlan(s site.Site, m Manifest, files bool, dbs []string, client string) (installer.Plan, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if m.Site != s.Name {
		return nil, fmt.Errorf("this backup is of site %q, not %q", m.Site, s.Name)
	}
	if !ValidStamp(m.Stamp) {
		return nil, fmt.Errorf("invalid backup stamp %q", m.Stamp)
	}
	if files && !m.Files {
		return nil, fmt.Errorf("backup %s has no files", m.Stamp)
	}
	if len(dbs) > 0 && !site.SafePath(client) {
		return nil, fmt.Errorf("invalid database client %q", client)
	}
	if err := checkDBs(s.Name, dbs); err != nil {
		return nil, err
	}
	for _, d := range dbs {
		if !contains(m.DBs, d) {
			return nil, fmt.Errorf("backup %s has no database %q (it has: %s)", m.Stamp, d, strings.Join(m.DBs, ", "))
		}
	}
	dir := Dir(s.Name, m.Stamp)
	var plan installer.Plan
	if files {
		f := dir + "/files.tar.gz"
		plan = append(plan, installer.Step{Name: "files", Actions: []installer.Action{installer.Run("sh", "-c", fmt.Sprintf(
			`[ -d %[2]s ] || { echo 'site root %[2]s does not exist; apply the site first' >&2; exit 1; }; gzip -t %[3]s && runuser -u %[1]s -- tar -C %[2]s -xzpf - < %[3]s`,
			s.User, s.Root, f))}})
	}
	for _, d := range dbs {
		f := fmt.Sprintf("%s/%s.sql.gz", dir, d)
		plan = append(plan, installer.Step{Name: "database " + d, Actions: []installer.Action{installer.Run("sh", "-c", fmt.Sprintf(
			`gzip -t %[2]s && gzip -dc %[2]s | %[1]s --default-character-set=utf8mb4 %[3]s`, client, f, d))}})
	}
	if len(plan) == 0 {
		return nil, fmt.Errorf("nothing to restore: choose the files, a database, or both")
	}
	return plan, nil
}

// Expired returns the stamps to delete so that only the newest keep remain. Names that are not stamps are ignored.
func Expired(stamps []string, keep int) []string {
	if keep < 1 {
		return nil
	}
	var ok []string
	for _, s := range stamps {
		if ValidStamp(s) {
			ok = append(ok, s)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ok)))
	if len(ok) <= keep {
		return nil
	}
	return ok[keep:]
}

// PrunePlan deletes the named backups of a site.
func PrunePlan(siteName string, stamps []string) (installer.Plan, error) {
	if !site.ValidName(siteName) {
		return nil, fmt.Errorf("invalid site name %q", siteName)
	}
	var acts []installer.Action
	for _, st := range stamps {
		if !ValidStamp(st) {
			return nil, fmt.Errorf("invalid backup stamp %q", st)
		}
		acts = append(acts, installer.Run("rm", "-rf", "--", Dir(siteName, st)))
	}
	return installer.Plan{{Name: "delete old backups", Actions: acts}}, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
