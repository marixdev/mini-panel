package main

import (
	"flag"
	"fmt"
	"strings"

	"mpn/internal/provision"
	"mpn/internal/tune"
)

// siteRemove takes a site off this machine, then deletes its database record. Without -apply it only prints the plan.
func siteRemove(args []string) {
	fs := flag.NewFlagSet("site remove", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	apply := fs.Bool("apply", false, "make the changes on this machine (Linux, root); otherwise the plan is only printed")
	purge := fs.Bool("purge", false, "also delete the system account and its home directory, which holds all site files")
	_ = fs.Parse(args)
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	dbs, err := st.ListDatabases(s.Name)
	fail(err)
	var dbNames []string
	for _, d := range dbs {
		dbNames = append(dbNames, d.Name)
	}
	if len(dbNames) > 0 && !*purge {
		fail(fmt.Errorf("site %q still has databases (%s): remove them with 'mpn db remove', or use -purge to drop them together with the site's files", s.Name, strings.Join(dbNames, ", ")))
	}
	plan, err := provision.RemovePlan(s, *purge)
	fail(err)
	if !*apply {
		fmt.Print(plan)
		if len(dbNames) > 0 {
			fmt.Printf("# would also DELETE the databases and their users: %s\n", strings.Join(dbNames, ", "))
		}
		fmt.Println("# dry run: nothing was changed. With -apply the site is removed and then its database record is deleted.")
		return
	}
	runApply(plan)
	if len(dbNames) > 0 {
		adm := dbAdmin()
		for _, n := range dbNames {
			fail(adm.Drop(n))
			fail(st.DeleteDatabase(n))
		}
	}
	fail(st.DeleteSite(*name))
	fmt.Printf("site %q removed\n", *name)
}

// siteApply builds the plan for a stored site. Without -apply it only prints the plan.
func siteApply(args []string) {
	fs := flag.NewFlagSet("site apply", flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	name := fs.String("name", "", "site name")
	apply := fs.Bool("apply", false, "make the changes on this machine (Linux, root); otherwise the plan is only printed")
	r := addRenderFlags(fs)
	_ = fs.Parse(args)
	if *apply {
		var cpus, swap int
		detectHost(fs, r.ram, &cpus, &swap)
	}
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*name)
	fail(err)
	plan, err := provision.Plan(s, provision.Options{
		Profile: tune.New(tune.Input{RAMMB: *r.ram, CPUs: 1}),
		TLS:     *r.tls, CertDir: *r.certDir, NodeMemMB: *r.nodeMem,
	})
	fail(err)
	if *apply {
		runApply(plan)
		return
	}
	fmt.Print(plan)
}
