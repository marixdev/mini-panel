package store

import (
	"fmt"
	"time"

	"mpn/internal/cron"
)

// CronJobs lists the scheduled jobs of a site in the order they were made.
func (s *Store) CronJobs(site string) ([]cron.Job, error) {
	rows, err := s.db.Query(`SELECT j.id, j.schedule, j.command, j.enabled FROM cron_jobs j JOIN sites s ON s.id = j.site_id WHERE s.name = ? ORDER BY j.id`, site)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []cron.Job{}
	for rows.Next() {
		var j cron.Job
		if err := rows.Scan(&j.ID, &j.Schedule, &j.Command, &j.Enabled); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// AddCronJob stores a job after checking it and the number of jobs the site has.
func (s *Store) AddCronJob(site, schedule, command string) (int64, error) {
	if err := cron.ValidSchedule(schedule); err != nil {
		return 0, err
	}
	if err := cron.ValidCommand(command); err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`SELECT id FROM sites WHERE name = ?`, site).Scan(&id); err != nil {
		return 0, fmt.Errorf("site %q: %w", site, ErrNotFound)
	}
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM cron_jobs WHERE site_id = ?`, id).Scan(&n); err != nil {
		return 0, err
	}
	if n >= cron.MaxJobs {
		return 0, fmt.Errorf("a site may have at most %d scheduled jobs", cron.MaxJobs)
	}
	res, err := tx.Exec(`INSERT INTO cron_jobs (site_id, schedule, command, enabled, created_at) VALUES (?,?,?,1,?)`, id, schedule, command, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	jid, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return jid, tx.Commit()
}

// SetCronEnabled switches a job on or off. The job must belong to the site.
func (s *Store) SetCronEnabled(site string, id int64, on bool) error {
	res, err := s.db.Exec(`UPDATE cron_jobs SET enabled = ? WHERE id = ? AND site_id = (SELECT id FROM sites WHERE name = ?)`, on, id, site)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("site %q has no scheduled job %d: %w", site, id, ErrNotFound)
	}
	return nil
}

// DeleteCronJob removes a job. The job must belong to the site.
func (s *Store) DeleteCronJob(site string, id int64) error {
	res, err := s.db.Exec(`DELETE FROM cron_jobs WHERE id = ? AND site_id = (SELECT id FROM sites WHERE name = ?)`, id, site)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("site %q has no scheduled job %d: %w", site, id, ErrNotFound)
	}
	return nil
}
