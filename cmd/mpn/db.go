package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"mpn/internal/dbadmin"
)

func runDB(args []string) {
	if len(args) == 0 {
		dbUsage()
	}
	switch args[0] {
	case "add":
		dbAdd(args[1:])
	case "list":
		dbList(args[1:])
	case "passwd":
		dbPasswd(args[1:])
	case "remove":
		dbRemove(args[1:])
	case "pma":
		dbPma(args[1:])
	case "pmadrop":
		dbPmaDrop(args[1:])
	case "pmaclean":
		dbPmaClean(args[1:])
	default:
		dbUsage()
	}
}

func dbUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn db <add|list|passwd|remove|pma|pmadrop|pmaclean> [flags]   (add -h for the flags)")
	os.Exit(2)
}

// dbAdmin returns the admin for the database server on this machine. Callers use it only after -apply was given.
func dbAdmin() dbadmin.Admin {
	requireLinuxRoot()
	client, err := dbadmin.Detect()
	fail(err)
	return dbadmin.Admin{R: dbadmin.ExecRunner{Client: client}}
}

func dbAdd(args []string) {
	fs := flag.NewFlagSet("db add", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	siteName := fs.String("site", "", "site the database belongs to")
	name := fs.String("name", "", "database and user name (default: the site name with - turned into _)")
	apply := fs.Bool("apply", false, "create it on the database server (Linux, root); otherwise only describe it")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	if _, err := st.GetSite(*siteName); err != nil {
		fail(err)
	}
	if *name == "" {
		*name = dbadmin.BaseName(*siteName)
	}
	if !dbadmin.ValidName(*name) {
		fail(fmt.Errorf("invalid name %q: use a-z, 0-9 and _, at most 32 characters", *name))
	}
	if !dbadmin.OwnedBy(*name, *siteName) {
		fail(fmt.Errorf("the name must be %q or start with %q so that it is clearly this site's", dbadmin.BaseName(*siteName), dbadmin.BaseName(*siteName)+"_"))
	}
	if !*apply {
		fmt.Printf("would create database %q and user %q for site %q, with all privileges on that database only and a generated password\n(dry run: add -apply to do it)\n", *name, *name, *siteName)
		return
	}
	adm := dbAdmin()
	pw, err := dbadmin.NewPassword()
	fail(err)
	// The record of a database can outlive the database: somebody dropped it with the mysql command or phpMyAdmin. The
	// record would then refuse the name for good. When the server has neither a database nor a user of that name, and the
	// record is this site's own, it is stale and goes.
	if old, gerr := st.GetDatabase(*name); gerr == nil && old.Site == *siteName {
		used, uerr := adm.InUse(*name)
		fail(uerr)
		if !used {
			fail(st.DeleteDatabase(*name))
			fmt.Printf("note: the record of %q was left from a database that no longer exists on the server; it was replaced\n", *name)
		}
	}
	fail(st.AddDatabase(*siteName, *name)) // first, so a name that is taken is refused before the server is touched
	if err := adm.Create(*name, pw); err != nil {
		_ = st.DeleteDatabase(*name)
		fail(err)
	}
	fmt.Printf("database created for site %q\n\n  host:     localhost (the server listens on this machine only)\n  database: %s\n  user:     %s\n  password: %s\n\n", *siteName, *name, *name, pw)
	fmt.Printf("Save the password now: mpn does not keep it anywhere.\nIf it is ever lost, set a new one with:  mpn db passwd -name %s -apply\n", *name)
}

func dbList(args []string) {
	fs := flag.NewFlagSet("db list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON instead of a table")
	db := fs.String("db", defaultDB, "mpn database path")
	siteName := fs.String("site", "", "only this site's databases")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	list, err := st.ListDatabases(*siteName)
	fail(err)
	if *asJSON {
		rows := []dbRow{}
		for _, d := range list {
			rows = append(rows, dbRow{Name: d.Name, Site: d.Site})
		}
		printJSON(rows)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "DATABASE\tSITE")
	for _, d := range list {
		fmt.Fprintf(w, "%s\t%s\n", d.Name, d.Site)
	}
	fail(w.Flush())
}

func dbPasswd(args []string) {
	fs := flag.NewFlagSet("db passwd", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	name := fs.String("name", "", "database (and user) name")
	stdin := fs.Bool("stdin", false, "read the new password from the first line of standard input instead of generating one")
	apply := fs.Bool("apply", false, "change it on the database server (Linux, root); otherwise only describe it")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	d, err := st.GetDatabase(*name)
	fail(err)
	if !*apply {
		fmt.Printf("would give user %q (site %q) a new password; the old one stops working at once\n(dry run: add -apply to do it)\n", d.Name, d.Site)
		return
	}
	adm := dbAdmin()
	var pw string
	if *stdin {
		line, rerr := bufio.NewReader(os.Stdin).ReadString('\n')
		pw = strings.TrimRight(line, "\r\n")
		if pw == "" {
			fail(errors.New("no password on standard input"))
		}
		_ = rerr
	} else {
		pw, err = dbadmin.NewPassword()
		fail(err)
	}
	fail(adm.SetPassword(d.Name, pw))
	if *stdin {
		fmt.Printf("password of %q changed\n", d.Name)
	} else {
		fmt.Printf("new password for %q:  %s\n\nSave it now: mpn does not keep it anywhere.\n", d.Name, pw)
	}
	fmt.Printf("The site's own settings (a config file, an .env file) still hold the old password and must be updated.\n")
}

func dbRemove(args []string) {
	fs := flag.NewFlagSet("db remove", flag.ExitOnError)
	db := fs.String("db", defaultDB, "mpn database path")
	name := fs.String("name", "", "database (and user) name")
	apply := fs.Bool("apply", false, "delete it on the database server (Linux, root); otherwise only describe it")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	d, err := st.GetDatabase(*name)
	fail(err)
	if !*apply {
		fmt.Printf("would DELETE database %q with all its data, and its user (site %q); this cannot be undone\n(dry run: add -apply to do it)\n", d.Name, d.Site)
		return
	}
	adm := dbAdmin()
	fail(adm.Drop(d.Name))
	fail(st.DeleteDatabase(d.Name))
	fmt.Printf("database %q and its user were deleted\n", d.Name)
}
