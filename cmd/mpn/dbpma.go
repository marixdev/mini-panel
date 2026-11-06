package main

import (
	"flag"
	"fmt"
	"time"

	"mpn/internal/dbadmin"
)

// The temporary users of the database browser (phpMyAdmin). See internal/dbadmin/temp.go for why they exist.

// pmaVisit is what "db pma" prints, as JSON, for the web panel to pass on to the browser.
type pmaVisit struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
	Expires  int64  `json:"expires"`
}

func dbPma(args []string) {
	fs := flag.NewFlagSet("db pma", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	name := fs.String("name", "", "database to open in the browser")
	apply := fs.Bool("apply", false, "make the user on the database server (Linux, root); otherwise only describe it")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	d, err := st.GetDatabase(*name) // only a database the panel manages can be opened
	fail(err)
	if !*apply {
		fmt.Printf("would make a temporary user for database %q (all privileges on that database only, valid for %s)\n(dry run: add -apply to do it)\n", d.Name, dbadmin.TempTTL)
		return
	}
	adm := dbAdmin()
	_, _ = adm.CleanTemp(time.Now()) // every visit also sweeps the visits that ended without a logout
	u, err := adm.CreateTemp(d.Name, time.Now())
	fail(err)
	printJSON(pmaVisit{User: u.Name, Password: u.Password, Database: d.Name, Expires: u.Expires.Unix()})
}

func dbPmaDrop(args []string) {
	fs := flag.NewFlagSet("db pmadrop", flag.ExitOnError)
	user := fs.String("user", "", "temporary user to drop")
	apply := fs.Bool("apply", false, "drop it on the database server (Linux, root); otherwise only describe it")
	_ = fs.Parse(args)
	if !dbadmin.ValidTempName(*user) {
		fail(fmt.Errorf("%q is not a temporary user", *user))
	}
	if !*apply {
		fmt.Printf("would drop the temporary user %q\n(dry run: add -apply to do it)\n", *user)
		return
	}
	fail(dbAdmin().DropTemp(*user))
	fmt.Printf("temporary user %q dropped\n", *user)
}

func dbPmaClean(args []string) {
	fs := flag.NewFlagSet("db pmaclean", flag.ExitOnError)
	apply := fs.Bool("apply", false, "drop them on the database server (Linux, root); otherwise only describe it")
	_ = fs.Parse(args)
	if !*apply {
		fmt.Println("would drop every temporary database-browser user whose time has passed\n(dry run: add -apply to do it)")
		return
	}
	dropped, err := dbAdmin().CleanTemp(time.Now())
	for _, n := range dropped {
		fmt.Printf("dropped %s\n", n)
	}
	fail(err)
	fmt.Printf("%d temporary user(s) dropped\n", len(dropped))
}
