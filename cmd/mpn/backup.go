package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"mpn/internal/backup"
	"mpn/internal/dbadmin"
	"mpn/internal/executor"
	"mpn/internal/installer"
	"mpn/internal/site"
	"mpn/internal/store"
)

func runBackup(args []string) {
	if len(args) == 0 {
		backupUsage()
	}
	switch args[0] {
	case "create":
		backupCreate(args[1:])
	case "list":
		backupList(args[1:])
	case "restore":
		backupRestore(args[1:])
	case "prune":
		backupPrune(args[1:])
	case "keep":
		backupKeep(args[1:])
	case "seal":
		backupSeal(args[1:])
	default:
		backupUsage()
	}
}

func backupUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn backup <create|list|restore|prune|keep> [flags]   (create -h for the flags)")
	os.Exit(2)
}

// applyPlan runs a plan and returns the error, so that a backup of one site failing does not stop the others.
func applyPlan(p installer.Plan) error {
	requireLinuxRoot()
	return executor.Apply(p, &executor.OS{Out: os.Stdout}, os.Stdout)
}

// sitesFor returns the one site named, or every site with -all.
func sitesFor(st *store.Store, name string, all bool) []site.Site {
	if all == (name != "") {
		fail(errors.New("give either -site NAME or -all"))
	}
	if all {
		l, err := st.ListSites()
		fail(err)
		return l
	}
	s, err := st.GetSite(name)
	fail(err)
	return []site.Site{s}
}

// listStamps returns a site's finished backups (those with a MANIFEST) and its unfinished ones, oldest first.
func listStamps(siteName string) (complete, partial []string) {
	entries, err := os.ReadDir(backup.SiteDir(siteName))
	if err != nil {
		return nil, nil
	}
	for _, e := range entries {
		if !e.IsDir() || !backup.ValidStamp(e.Name()) {
			continue
		}
		if _, err := os.Stat(backup.Dir(siteName, e.Name()) + "/MANIFEST"); err == nil {
			complete = append(complete, e.Name())
		} else {
			partial = append(partial, e.Name())
		}
	}
	sort.Strings(complete)
	sort.Strings(partial)
	return
}

func readManifest(siteName, stamp string) (backup.Manifest, error) {
	b, err := os.ReadFile(backup.Dir(siteName, stamp) + "/MANIFEST")
	if err != nil {
		return backup.Manifest{}, fmt.Errorf("backup %s of site %q: %w", stamp, siteName, err)
	}
	return backup.ParseManifest(string(b))
}

// backupSeal is the last step of a backup: it sums the files and writes the MANIFEST signed with the server's key. It is a step
// of the plan and not meant to be run by hand.
func backupSeal(args []string) {
	fs := flag.NewFlagSet("backup seal", flag.ExitOnError)
	dir := fs.String("dir", "", "folder of the backup")
	name := fs.String("site", "", "site")
	stamp := fs.String("stamp", "", "name of the backup")
	htdocs := fs.Bool("htdocs", false, "files.tar.gz holds the whole htdocs")
	var dbs multiFlag
	fs.Var(&dbs, "dbname", "a database in the backup (repeat)")
	_ = fs.Parse(args)
	requireLinuxRoot()
	if !site.ValidName(*name) || !backup.ValidStamp(*stamp) || *dir != backup.Dir(*name, *stamp) {
		fail(errors.New("backup seal: the folder must be the backup of -site at -stamp"))
	}
	for _, d := range dbs {
		if !dbadmin.ValidName(d) || !dbadmin.OwnedBy(d, *name) {
			fail(fmt.Errorf("backup seal: %q is not a database of %q", d, *name))
		}
	}
	key, err := backup.LoadKey(backup.KeyFile)
	fail(err)
	fail(backup.Seal(*dir, backup.Manifest{Site: *name, Stamp: *stamp, Files: true, Htdocs: *htdocs, DBs: dbs}, key))
}

// multiFlag is a flag that may be given more than once.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func dirSizeMB(dir string) float64 {
	var n int64
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && fi.Mode().IsRegular() {
			n += fi.Size()
		}
	}
	return float64(n) / (1 << 20)
}

func backupCreate(args []string) {
	fs := flag.NewFlagSet("backup create", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	name := fs.String("site", "", "site to back up")
	all := fs.Bool("all", false, "back up every site")
	keep := fs.Int("keep", 0, "afterwards delete all but this many newest backups of each site (0 keeps everything)")
	apply := fs.Bool("apply", false, "make the backup (Linux, root); otherwise only the plan is printed")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	sites := sitesFor(st, *name, *all)
	stamp := time.Now().Format("20060102-150405")
	failed := 0
	report := func(s site.Site, err error) {
		fmt.Fprintf(os.Stderr, "site %s: %v\n", s.Name, err)
		failed++
	}
	for _, s := range sites {
		dbs, err := st.ListDatabases(s.Name)
		if err != nil {
			report(s, err)
			continue
		}
		var names []string
		for _, d := range dbs {
			names = append(names, d.Name)
		}
		dump := "/usr/bin/mysqldump" // only shown in a dry run
		if *apply && len(names) > 0 {
			requireLinuxRoot()
			if dump, err = dbadmin.DetectDump(); err != nil {
				report(s, err)
				continue
			}
		}
		plan, err := backup.CreatePlan(s, stamp, names, dump)
		if err != nil {
			report(s, err)
			continue
		}
		if !*apply {
			fmt.Printf("# site %s\n%s", s.Name, plan)
			continue
		}
		dir := backup.Dir(s.Name, stamp)
		_, statErr := os.Stat(dir)
		if statErr == nil {
			report(s, fmt.Errorf("a backup named %s already exists (another backup ran in the same second); wait a moment and run it again", stamp))
			continue
		}
		fmt.Printf("backing up %s\n", s.Name)
		if err := applyPlan(plan); err != nil {
			if statErr != nil { // this run made the directory, so the half-finished backup goes away with it
				_ = os.RemoveAll(dir)
			}
			report(s, err)
			continue
		}
		fmt.Printf("saved %s (%.1f MB)\n", dir, dirSizeMB(dir))
		if n := keepFor(st, s.Name, *keep); n > 0 {
			if err := pruneSite(s.Name, n); err != nil {
				report(s, err)
			}
		}
	}
	if !*apply {
		fmt.Println("# dry run: nothing was changed. Add -apply to make the backup.")
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func backupList(args []string) {
	fs := flag.NewFlagSet("backup list", flag.ExitOnError)
	name := fs.String("site", "", "only this site's backups")
	asJSON := fs.Bool("json", false, "print JSON instead of a table")
	_ = fs.Parse(args)
	names := []string{*name}
	if *name == "" {
		names = nil
		entries, _ := os.ReadDir(backup.Base)
		for _, e := range entries {
			if e.IsDir() && site.ValidName(e.Name()) {
				names = append(names, e.Name())
			}
		}
	} else if !site.ValidName(*name) {
		fail(fmt.Errorf("invalid site name %q", *name))
	}
	rows := []backupRow{}
	for _, n := range names {
		complete, partial := listStamps(n)
		for _, s := range partial {
			rows = append(rows, backupRow{Site: n, Backup: s, State: "incomplete", Databases: []string{}})
		}
		for _, s := range complete {
			m, err := readManifest(n, s)
			if err != nil {
				rows = append(rows, backupRow{Site: n, Backup: s, State: "unreadable", Databases: []string{}})
				continue
			}
			dbs := append([]string{}, m.DBs...)
			rows = append(rows, backupRow{Site: n, Backup: s, State: "ok", Files: m.Files, Databases: dbs, SizeMB: dirSizeMB(backup.Dir(n, s))})
		}
	}
	if *asJSON {
		printJSON(rows)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SITE\tBACKUP\tSTATE\tFILES\tDATABASES\tSIZE")
	for _, r := range rows {
		files := "no"
		if r.Files {
			files = "yes"
		}
		if r.State != "ok" {
			fmt.Fprintf(w, "%s\t%s\t%s\t\t\t\n", r.Site, r.Backup, strings.ToUpper(r.State))
			continue
		}
		fmt.Fprintf(w, "%s\t%s\tok\t%s\t%s\t%.1f MB\n", r.Site, r.Backup, files, strings.Join(r.Databases, ","), r.SizeMB)
	}
	fail(w.Flush())
}

func backupRestore(args []string) {
	fs := flag.NewFlagSet("backup restore", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	name := fs.String("site", "", "site to restore")
	stamp := fs.String("backup", "latest", "which backup (see 'mpn backup list'), or latest")
	files := fs.Bool("files", false, "restore the site files")
	all := fs.Bool("all", false, "restore everything in the backup (files and all databases)")
	var dbs hostList
	fs.Var(&dbs, "database", "restore this database, repeat for several")
	apply := fs.Bool("apply", false, "restore (Linux, root); otherwise only the plan is printed")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	if *stamp == "latest" {
		complete, _ := listStamps(s.Name)
		if len(complete) == 0 {
			fail(fmt.Errorf("site %q has no finished backup", s.Name))
		}
		*stamp = complete[len(complete)-1]
	}
	if !backup.ValidStamp(*stamp) {
		fail(fmt.Errorf("invalid backup name %q", *stamp))
	}
	m, err := readManifest(s.Name, *stamp)
	fail(err)
	if *all {
		*files, dbs = m.Files, m.DBs
	}
	for _, d := range dbs {
		rec, err := st.GetDatabase(d)
		fail(err)
		if rec.Site != s.Name {
			fail(fmt.Errorf("database %q belongs to site %q, not %q", d, rec.Site, s.Name))
		}
	}
	client := "/usr/bin/mysql" // only shown in a dry run
	if *apply && len(dbs) > 0 {
		requireLinuxRoot()
		client, err = dbadmin.Detect()
		fail(err)
	}
	plan, err := backup.RestorePlan(s, m, *files, dbs, client)
	fail(err)
	if !*apply {
		fmt.Print(plan)
		fmt.Println("# dry run: nothing was changed. The restore overwrites the files that are in the backup and the tables in the dumps.")
		return
	}
	fail(applyPlan(plan))
	fmt.Printf("restored backup %s of site %q\n", *stamp, s.Name)
}

// The number of backups kept of a site is chosen on its Backups tab and saved in the settings. A site without one keeps what the
// caller asks: 7 for the nightly backup (its unit says -keep 7), all of them for a backup made by hand on a server where the number
// was never chosen. The copy off the server keeps the same number.
const (
	defaultBackupKeep = 7
	maxBackupKeep     = 365
)

func backupKeepKey(siteName string) string { return "backup.keep." + siteName }

// keepFor is the number of backups to keep of a site: its own setting, or def when it has none.
func keepFor(st *store.Store, siteName string, def int) int {
	v, _ := st.Setting(backupKeepKey(siteName), "")
	if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= maxBackupKeep {
		return n
	}
	return def
}

// backupKeep shows or changes the number of backups kept of a site. A smaller number deletes the backups beyond it at once.
func backupKeep(args []string) {
	fs := flag.NewFlagSet("backup keep", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	name := fs.String("site", "", "site")
	n := fs.Int("n", 0, "number of newest backups to keep, 1 to 365; without it the current number is shown")
	asJSON := fs.Bool("json", false, "show the number as JSON")
	apply := fs.Bool("apply", false, "save the number and delete the backups beyond it (Linux, root)")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	if *n == 0 {
		cur := keepFor(st, s.Name, defaultBackupKeep)
		if *asJSON {
			fmt.Printf("{\"keep\":%d}\n", cur)
			return
		}
		fmt.Printf("site %s keeps %d backup(s)\n", s.Name, cur)
		return
	}
	if *n < 1 || *n > maxBackupKeep {
		fail(fmt.Errorf("-n must be between 1 and %d", maxBackupKeep))
	}
	complete, _ := listStamps(s.Name)
	old := backup.Expired(complete, *n)
	if !*apply {
		fmt.Printf("site %s would keep %d backup(s) and delete %d: %s\n", s.Name, *n, len(old), strings.Join(old, ", "))
		fmt.Println("# dry run: nothing was changed. Add -apply to save it.")
		return
	}
	fail(st.SetSetting(backupKeepKey(s.Name), strconv.Itoa(*n)))
	fail(pruneSite(s.Name, *n))
	fmt.Printf("site %s keeps the %d newest backup(s)\n", s.Name, *n)
}

// pruneSite keeps the newest `keep` finished backups of a site and deletes the rest.
func pruneSite(siteName string, keep int) error {
	complete, _ := listStamps(siteName)
	old := backup.Expired(complete, keep)
	if len(old) == 0 {
		return nil
	}
	p, err := backup.PrunePlan(siteName, old)
	if err != nil {
		return err
	}
	fmt.Printf("deleting %d old backup(s) of %s: %s\n", len(old), siteName, strings.Join(old, ", "))
	return applyPlan(p)
}

func backupPrune(args []string) {
	fs := flag.NewFlagSet("backup prune", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	name := fs.String("site", "", "site whose old backups to delete")
	all := fs.Bool("all", false, "every site")
	keep := fs.Int("keep", 0, "number of newest backups to keep (at least 1)")
	apply := fs.Bool("apply", false, "delete them (Linux, root); otherwise only list them")
	_ = fs.Parse(args)
	if *keep < 1 {
		fail(errors.New("-keep must be at least 1"))
	}
	st := openStore(*db)
	defer st.Close()
	for _, s := range sitesFor(st, *name, *all) {
		complete, _ := listStamps(s.Name)
		old := backup.Expired(complete, *keep)
		if !*apply {
			fmt.Printf("site %s: would delete %d backup(s) %s\n", s.Name, len(old), strings.Join(old, ", "))
			continue
		}
		fail(pruneSite(s.Name, *keep))
	}
	if !*apply {
		fmt.Println("# dry run: nothing was deleted. Add -apply to delete.")
	}
}
