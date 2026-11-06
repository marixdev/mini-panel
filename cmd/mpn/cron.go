package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"mpn/internal/cron"
	"mpn/internal/installer"
)

func cronUsage() {
	fmt.Fprintln(os.Stderr, "usage: mpn cron <list|add|enable|disable|remove|sync> -site NAME [flags]   (add -h for the flags)")
	os.Exit(2)
}

// runCron manages the scheduled jobs of a website. A job runs as the website's own account; mpn keeps them in its database
// and writes the enabled ones to /etc/cron.d/mpn-<site> (root's file, mode 0600, because a command often holds a password).
// The file is written first and the database after, so what the database says is what runs.
func runCron(args []string) {
	if len(args) == 0 {
		cronUsage()
	}
	sub := args[0]
	fs := flag.NewFlagSet("cron "+sub, flag.ExitOnError)
	db := fs.String("db", defaultDB, "database path")
	site := fs.String("site", "", "site name")
	id := fs.Int64("id", 0, "job number, for enable, disable and remove")
	stdin := fs.Bool("stdin", false, "add: read {schedule, command} as JSON on standard input, so the command is not an argument")
	schedule := fs.String("schedule", "", "add: five fields, or @daily and the like")
	command := fs.String("command", "", "add: the command line")
	asJSON := fs.Bool("json", false, "list: print JSON")
	apply := fs.Bool("apply", false, "change this machine (Linux, root); otherwise only check")
	_ = fs.Parse(args[1:])
	st := openStore(*db)
	defer st.Close()
	s, err := st.GetSite(*site)
	fail(err)
	jobs, err := st.CronJobs(s.Name)
	fail(err)

	switch sub {
	case "list":
		if *asJSON {
			printJSON(jobs)
			return
		}
		for _, j := range jobs {
			fmt.Printf("%d\t%v\t%s\t%s\n", j.ID, j.Enabled, j.Schedule, j.Command)
		}
		return
	case "sync":
		// writes the file again from the database, for a machine where it was lost
	case "add":
		if *stdin {
			raw, err := io.ReadAll(io.LimitReader(os.Stdin, 8<<10))
			fail(err)
			var in struct{ Schedule, Command string }
			if err := json.Unmarshal(raw, &in); err != nil {
				fail(fmt.Errorf("the input is not the JSON that mpn expects: %w", err))
			}
			*schedule, *command = in.Schedule, in.Command
		}
		fail(cron.ValidSchedule(*schedule))
		fail(cron.ValidCommand(*command))
		if len(jobs) >= cron.MaxJobs {
			fail(fmt.Errorf("a site may have at most %d scheduled jobs", cron.MaxJobs))
		}
		jobs = append(jobs, cron.Job{ID: -1, Schedule: *schedule, Command: *command, Enabled: true})
	case "enable", "disable", "remove":
		found := false
		out := jobs[:0:0]
		for _, j := range jobs {
			if j.ID != *id {
				out = append(out, j)
				continue
			}
			found = true
			if sub != "remove" {
				j.Enabled = sub == "enable"
				out = append(out, j)
			}
		}
		if !found {
			fail(fmt.Errorf("site %q has no scheduled job %d", s.Name, *id))
		}
		jobs = out
	default:
		cronUsage()
	}

	text, err := cron.Render(s.Name, s.User, jobs)
	fail(err)
	if !*apply {
		fmt.Printf("# checked only. The file %s would be:\n%s", cron.File(s.Name), text)
		return
	}
	requireLinuxRoot()
	fail(runPlanQuiet(cronPlan(s.Name, text)))
	switch sub {
	case "add":
		n, err := st.AddCronJob(s.Name, *schedule, *command)
		fail(err)
		fmt.Printf("job %d added to site %q\n", n, s.Name)
	case "enable", "disable":
		fail(st.SetCronEnabled(s.Name, *id, sub == "enable"))
		fmt.Printf("job %d of site %q is %sd\n", *id, s.Name, sub)
	case "remove":
		fail(st.DeleteCronJob(s.Name, *id))
		fmt.Printf("job %d removed from site %q\n", *id, s.Name)
	default:
		fmt.Printf("%s written again\n", cron.File(s.Name))
	}
}

// cronPlan writes the cron file of a site, or removes it when no job is enabled.
func cronPlan(site, text string) installer.Plan {
	if text == "" {
		return installer.Plan{{Name: "cron file", Actions: []installer.Action{installer.Run("rm", "-f", cron.File(site))}}}
	}
	return installer.Plan{{Name: "cron file", Actions: []installer.Action{installer.WriteFile(cron.File(site), text, 0o600)}}}
}
