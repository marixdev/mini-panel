// Package cron checks and writes the scheduled jobs of a website. A job runs as the website's own account. mpn writes them to
// /etc/cron.d/mpn-<site>, a file owned by root, so every value that lands in it is checked first: one stray line break in a
// command would add a line that runs as another user.
package cron

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxJobs is how many jobs a site may have; MaxCommand is the longest command.
const (
	MaxJobs    = 20
	MaxCommand = 1000
)

// Job is one scheduled command.
type Job struct {
	ID       int64  `json:"id"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
}

var macros = map[string]bool{"@hourly": true, "@daily": true, "@midnight": true, "@weekly": true, "@monthly": true, "@yearly": true, "@annually": true}

// fields are the five parts of a schedule with the numbers each may take.
var fields = [5]struct {
	name     string
	min, max int
}{{"minute", 0, 59}, {"hour", 0, 23}, {"day of month", 1, 31}, {"month", 1, 12}, {"day of week", 0, 7}}

var itemRe = regexp.MustCompile(`^(\*|\d{1,2}(-\d{1,2})?)(/\d{1,2})?$`)

func checkField(i int, f string) error {
	if f == "" {
		return fmt.Errorf("the %s is empty", fields[i].name)
	}
	for _, item := range strings.Split(f, ",") {
		m := itemRe.FindStringSubmatch(item)
		if m == nil {
			return fmt.Errorf("the %s %q is not understood; use numbers, ranges like 1-5, lists like 1,15 and steps like */5", fields[i].name, item)
		}
		base := item
		if k := strings.IndexByte(item, '/'); k >= 0 {
			base = item[:k]
			if step, _ := strconv.Atoi(item[k+1:]); step < 1 {
				return fmt.Errorf("the step of the %s must be at least 1", fields[i].name)
			}
		}
		if base == "*" {
			continue
		}
		lo, hi := base, base
		if k := strings.IndexByte(base, '-'); k >= 0 {
			lo, hi = base[:k], base[k+1:]
		}
		a, _ := strconv.Atoi(lo)
		b, _ := strconv.Atoi(hi)
		if a < fields[i].min || b > fields[i].max || a > b {
			return fmt.Errorf("the %s must be between %d and %d", fields[i].name, fields[i].min, fields[i].max)
		}
	}
	return nil
}

// ValidSchedule checks a schedule: five fields (minute, hour, day of month, month, day of week) or @hourly, @daily, @weekly,
// @monthly, @yearly. Names such as "mon" are not accepted; numbers are enough and leave nothing to misread.
func ValidSchedule(s string) error {
	if strings.HasPrefix(s, "@") {
		if !macros[s] {
			return fmt.Errorf("%q is not one of @hourly, @daily, @weekly, @monthly, @yearly", s)
		}
		return nil
	}
	parts := strings.Fields(s)
	if len(parts) != 5 || strings.Join(parts, " ") != s {
		return fmt.Errorf("a schedule has five fields separated by one space: minute hour day month weekday")
	}
	for i, p := range parts {
		if err := checkField(i, p); err != nil {
			return err
		}
	}
	return nil
}

// ValidCommand checks a command. It is one line: a line break would end the cron line and start another one.
func ValidCommand(c string) error {
	switch {
	case strings.TrimSpace(c) == "":
		return fmt.Errorf("the command is empty")
	case len(c) > MaxCommand:
		return fmt.Errorf("the command is longer than %d characters", MaxCommand)
	case !utf8.ValidString(c) || strings.ContainsAny(c, "\r\n\x00"):
		return fmt.Errorf("the command must be one line of text")
	case c != strings.TrimSpace(c):
		return fmt.Errorf("the command must not start or end with a space")
	}
	return nil
}

// escape makes a command safe for a cron line: cron turns every % into a line break unless it is written \%.
func escape(c string) string { return strings.ReplaceAll(c, "%", `\%`) }

var userRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// Render makes the content of /etc/cron.d/mpn-<site> from the enabled jobs, or "" when there are none. site and user have
// passed the validation of site names and accounts.
func Render(site, user string, jobs []Job) (string, error) {
	if !userRe.MatchString(user) {
		return "", fmt.Errorf("account %q is not valid", user)
	}
	var b strings.Builder
	n := 0
	for _, j := range jobs {
		if !j.Enabled {
			continue
		}
		if err := ValidSchedule(j.Schedule); err != nil {
			return "", err
		}
		if err := ValidCommand(j.Command); err != nil {
			return "", err
		}
		n++
		fmt.Fprintf(&b, "%s %s %s\n", j.Schedule, user, escape(j.Command))
	}
	if n == 0 {
		return "", nil
	}
	head := "# Made by mpn for the website " + site + ". Changes made here are overwritten; use the panel.\nSHELL=/bin/sh\nPATH=/usr/local/bin:/usr/bin:/bin\nMAILTO=\"\"\n"
	return head + b.String(), nil
}

// File is the path of a site's cron file. Names with a dot are ignored by cron, and a site name has none.
func File(site string) string { return "/etc/cron.d/mpn-" + site }
