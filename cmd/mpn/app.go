package main

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"mpn/internal/apps"
	"mpn/internal/dbadmin"
	"mpn/internal/installer"
	"mpn/internal/provision"
	"mpn/internal/site"
	"mpn/internal/store"
	"mpn/internal/tune"
)

func appUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn app install -site NAME -app wordpress|laravel|codeigniter|cakephp|yii|phpbb [flags]   (add -h for the flags)")
	fmt.Fprintln(os.Stderr, "       mpn app phpbb-check [-apply]   close the phpBB installer of sites that have an administrator")
	os.Exit(2)
}

// appDBName picks the name of the database an application gets: the site's own name, or that with _app when the first is taken. A name is
// taken when mpn has a record of it or the database server has a database or a user of that name.
func appDBName(st *store.Store, siteName string, adm *dbadmin.Admin) (string, error) {
	base := dbadmin.BaseName(siteName)
	for _, n := range []string{base, base + "_app"} {
		if !dbadmin.ValidName(n) || !dbadmin.OwnedBy(n, siteName) {
			continue
		}
		if _, err := st.GetDatabase(n); err == nil {
			continue
		}
		if adm != nil {
			if used, err := adm.InUse(n); err != nil {
				return "", err
			} else if used {
				continue
			}
		}
		return n, nil
	}
	return "", fmt.Errorf("app-db: the database names %s and %s_app are both in use; remove one first", base, base)
}

// mergeIni adds the settings that suit an application to the settings a person already chose, without changing a value the person set.
func mergeIni(have, want map[string]string) map[string]string {
	out := maps.Clone(have)
	for k, v := range want {
		if _, set := out[k]; set {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[k] = v
	}
	return out
}

func appInstall(args []string) {
	fs := flag.NewFlagSet("app install", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	siteName := fs.String("site", "", "the website")
	appID := fs.String("app", "", "the application: "+fmt.Sprint(apps.IDs()))
	title := fs.String("title", "", "wordpress: the title of the site")
	admin := fs.String("admin", "", "wordpress: the name of the administrator")
	email := fs.String("email", "", "wordpress: the email of the administrator")
	locale := fs.String("locale", "en_US", "wordpress: the language")
	ramFlag := fs.Int("ram", 1024, "host RAM in MB, sizes the PHP pool")
	apply := fs.Bool("apply", false, "download and install it (Linux, root); otherwise only the plan is printed")
	_ = fs.Parse(args)

	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*siteName)
	fail(err)
	recipe, ok := apps.Get(*appID)
	if !ok {
		fail(fmt.Errorf("-app %q is not one of %v", *appID, apps.IDs()))
	}
	if s.Kind != site.PHP {
		fail(errors.New("app-kind: applications are installed in PHP sites only"))
	}
	if !slices.Contains(site.InstalledPHP(), s.PHPVersion) && *apply {
		fail(fmt.Errorf("php %s is not installed on this machine", s.PHPVersion))
	}

	p := apps.Params{Site: s, Recipe: recipe, Title: *title, AdminUser: *admin, AdminEmail: *email, Locale: *locale}
	var adm *dbadmin.Admin
	if recipe.NeedsDB() {
		if *apply {
			a := dbAdmin()
			adm = &a
		}
		p.DBName, err = appDBName(st, s.Name, adm)
		fail(err)
		p.DBUser = p.DBName
		if p.DBPass, err = dbadmin.NewPassword(); err != nil {
			fail(err)
		}
	}
	if recipe.ID == apps.WordPressID {
		if p.AdminPass, err = dbadmin.NewPassword(); err != nil {
			fail(err)
		}
	}
	prep, install, err := apps.Plan(p)
	fail(err)
	if !*apply {
		fmt.Print(prep)
		fmt.Print(install)
		fmt.Println("# dry run: nothing was changed. Add -apply to download and install the application.")
		return
	}
	requireLinuxRoot()

	fmt.Printf("# installing %s in %s\n", recipe.Label, s.Name)
	fail(runPlanQuiet(prep)) // tools and an empty web root; nothing here changes what a person put in the web root
	created := false
	cleanup := func() {
		_ = runPlanQuiet(apps.CleanupPlan(p))
		if created {
			_ = adm.Drop(p.DBName)
			_ = st.DeleteDatabase(p.DBName)
		}
	}
	if recipe.NeedsDB() {
		fail(st.AddDatabase(s.Name, p.DBName))
		if err := adm.Create(p.DBName, p.DBPass); err != nil {
			_ = st.DeleteDatabase(p.DBName)
			fail(err)
		}
		created = true
	}
	if err := runPlanQuiet(install); err != nil {
		cleanup()
		fail(fmt.Errorf("the installation of %s failed and was undone: %w", recipe.Label, err))
	}

	// The files are in place; now the site is told where its web root is and which settings suit the application.
	cur := s
	cur.App = recipe.ID
	cur.Root = site.RootFor(s.User, recipe.DocRoot)
	cur.PHPIni = mergeIni(s.PHPIni, recipe.PHPIni)
	if err := cur.Validate(); err != nil {
		cleanup()
		fail(err)
	}
	plan, err := provision.ChangePlan(s, cur, provision.Options{Profile: tune.New(tune.Input{RAMMB: *ramFlag, CPUs: 1}), NodeMemMB: 256})
	fail(err)
	fail(runPlanQuiet(plan))
	fail(st.UpdateSite(cur))
	fail(st.SetApp(cur.Name, recipe.ID))

	if recipe.ID == apps.PHPBBID {
		fail(runPlanQuiet(phpbbTimerPlan()))
		fmt.Printf("\nphpBB %s is in %q. Finish the setup in the browser:\n\n  installer:  %s/install/\n  database:   MySQL with MySQLi, server localhost, port empty\n  name:       %s\n  user:       %s\n  password:   %s\n  prefix:     phpbb_ (or your own)\n\nOnce the administrator is made, the panel closes /install/ and removes the folder within a minute.\n",
			apps.PHPBBVersion, s.Name, p.URL(), p.DBName, p.DBUser, p.DBPass)
		return
	}
	fmt.Printf("\n%s is installed in %q\n\n  address:    %s\n  web root:   %s\n", recipe.Label, s.Name, p.URL(), cur.Root)
	if recipe.NeedsDB() {
		fmt.Printf("  database:   %s (user %s; the password is in %s)\n", p.DBName, p.DBUser, map[bool]string{true: "wp-config.php", false: ".env"}[recipe.ID == apps.WordPressID])
	}
	if recipe.ID == apps.WordPressID {
		fmt.Printf("  admin page: %s/wp-admin/\n  admin:      %s\n  password:   %s\n\nSave the password now: mpn does not keep it anywhere. Change it in WordPress if it is ever lost.\n", p.URL(), p.AdminUser, p.AdminPass)
	}
}

func runApp(args []string) {
	switch {
	case len(args) > 0 && args[0] == "install":
		appInstall(args[1:])
	case len(args) > 0 && args[0] == "phpbb-check":
		phpbbCheck(args[1:])
	default:
		appUsage()
	}
}

const (
	phpbbTimerName = "mpn-phpbb-check.timer"
	phpbbService   = "[Unit]\nDescription=mpn closes the phpBB installer once an administrator exists\n\n[Service]\nType=oneshot\nExecStart=/usr/local/bin/mpn app phpbb-check -apply\n"
	phpbbTimer     = "[Unit]\nDescription=mpn check of phpBB sites still being installed\n\n[Timer]\nOnBootSec=2min\nOnUnitActiveSec=1min\n\n[Install]\nWantedBy=timers.target\n"
)

// phpbbTimerPlan starts the check that closes the installer; it stops itself once no phpBB site waits.
func phpbbTimerPlan() installer.Plan {
	return installer.Plan{{Name: "phpBB check", Actions: []installer.Action{
		installer.WriteFile("/etc/systemd/system/mpn-phpbb-check.service", phpbbService, 0o644),
		installer.WriteFile("/etc/systemd/system/"+phpbbTimerName, phpbbTimer, 0o644),
		installer.Run("systemctl", "daemon-reload"),
		installer.Run("systemctl", "enable", "--now", phpbbTimerName),
	}}}
}

// phpbbCheck looks at each phpBB site whose installer is still open. When its config.php says the installer finished and the database
// has a founder, the vhost is written without the installer's location, the install folder is removed and the site is marked ready.
func phpbbCheck(args []string) {
	fs := flag.NewFlagSet("app phpbb-check", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	apply := fs.Bool("apply", false, "close the installers that are done (Linux, root); otherwise only report")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	sites, err := st.ListSites()
	fail(err)
	waiting := 0
	for _, s := range sites {
		if s.App != apps.PHPBBID || s.AppReady {
			continue
		}
		done, why := phpbbDone(s, *apply)
		if !done {
			waiting++
			fmt.Printf("%s: waiting (%s)\n", s.Name, why)
			continue
		}
		if !*apply {
			fmt.Printf("%s: installed; -apply closes the installer\n", s.Name)
			continue
		}
		cur := s
		cur.AppReady = true
		plan, err := provision.VhostPlan(cur, "")
		if err == nil {
			err = runPlanQuiet(append(plan, apps.PHPBBFinishPlan(cur)...))
		}
		if err == nil {
			err = st.SetAppReady(s.Name, true)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", s.Name, err)
			waiting++
			continue
		}
		fmt.Printf("%s: installed; the installer is closed and install/ removed\n", s.Name)
	}
	if waiting == 0 && *apply {
		_ = runPlanQuiet(installer.Plan{{Name: "phpBB check", Actions: []installer.Action{installer.Run("sh", "-c", "systemctl disable --now "+phpbbTimerName+" 2>/dev/null || true")}}})
	}
}

// phpbbDone reads config.php of the site and asks the database for a founder. Without apply the database is not asked.
func phpbbDone(s site.Site, apply bool) (bool, string) {
	b, err := os.ReadFile(filepath.Join(site.HtdocsOf(s.User), "config.php"))
	if err != nil {
		return false, "no config.php"
	}
	name, prefix, ok := apps.PHPBBConfig(string(b))
	if !ok {
		return false, "the installer has not finished"
	}
	if !dbadmin.OwnedBy(name, s.Name) {
		return false, "config.php names a database of another site"
	}
	if !apply {
		return true, ""
	}
	client, err := dbadmin.Detect()
	if err != nil {
		return false, err.Error()
	}
	out, err := dbadmin.ExecRunner{Client: client}.Run(apps.PHPBBFounderSQL(name, prefix))
	if err != nil {
		return false, "the tables are not there yet"
	}
	if n, _ := strconv.Atoi(strings.TrimSpace(out)); n < 1 {
		return false, "no administrator yet"
	}
	// The founder is written in the middle of the run: the installer goes on for a while (migrations, packaged extensions, the
	// style), still loading its files from install/. Removing the folder then broke the rest with "The ../styles/prosilver/template
	// directory does not exist" (seen 2026-11-23). store/install_config.php holds the run's progress and is removed only when it ends.
	if _, err := os.Stat(filepath.Join(site.HtdocsOf(s.User), apps.PHPBBProgressFile)); err == nil {
		return false, "the installer is still running"
	}
	return true, ""
}
