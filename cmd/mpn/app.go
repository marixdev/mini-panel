package main

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"

	"mpn/internal/apps"
	"mpn/internal/dbadmin"
	"mpn/internal/provision"
	"mpn/internal/site"
	"mpn/internal/store"
	"mpn/internal/tune"
)

func appUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn app install -site NAME -app wordpress|laravel|codeigniter|cakephp|yii [flags]   (add -h for the flags)")
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
	if !slices.Contains(site.InstalledPHP("/usr/sbin"), s.PHPVersion) && *apply {
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

	fmt.Printf("\n%s is installed in %q\n\n  address:    %s\n  web root:   %s\n", recipe.Label, s.Name, p.URL(), cur.Root)
	if recipe.NeedsDB() {
		fmt.Printf("  database:   %s (user %s; the password is in %s)\n", p.DBName, p.DBUser, map[bool]string{true: "wp-config.php", false: ".env"}[recipe.ID == apps.WordPressID])
	}
	if recipe.ID == apps.WordPressID {
		fmt.Printf("  admin page: %s/wp-admin/\n  admin:      %s\n  password:   %s\n\nSave the password now: mpn does not keep it anywhere. Change it in WordPress if it is ever lost.\n", p.URL(), p.AdminUser, p.AdminPass)
	}
}

func runApp(args []string) {
	if len(args) == 0 || args[0] != "install" {
		appUsage()
	}
	appInstall(args[1:])
}
