package store

import (
	"fmt"
	"time"

	"mpn/internal/background"
	sitepkg "mpn/internal/site"
)

// BgServices lists the background services of a site in the order they were made.
func (s *Store) BgServices(site string) ([]background.Service, error) {
	rows, err := s.db.Query(`SELECT b.id, b.name, b.args, b.enabled, b.memory_mb FROM bg_services b JOIN sites s ON s.id = b.site_id WHERE s.name = ? ORDER BY b.id`, site)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []background.Service{}
	for rows.Next() {
		var v background.Service
		if err := rows.Scan(&v.ID, &v.Name, &v.Args, &v.Enabled, &v.MemoryMB); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// NextBgServiceID is the number the next service will get: one more than the largest number in use. The CLI needs it before
// the service is stored, because the unit file is written first and its name carries the number.
func (s *Store) NextBgServiceID() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT COALESCE(MAX(id), 0) + 1 FROM bg_services`).Scan(&n)
	return n, err
}

// AddBgService stores a service under the number id, after checking it and the number of services the site has. The kind of
// the site decides what the arguments may hold, so the caller passes the site's kind.
func (s *Store) AddBgService(site string, kind sitepkg.Kind, id int64, in background.Input) error {
	if err := background.ValidService(kind, in); err != nil {
		return err
	}
	if id < 1 {
		return fmt.Errorf("service number %d is not valid", id)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var siteID int64
	if err := tx.QueryRow(`SELECT id FROM sites WHERE name = ?`, site).Scan(&siteID); err != nil {
		return fmt.Errorf("site %q: %w", site, ErrNotFound)
	}
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM bg_services WHERE site_id = ?`, siteID).Scan(&n); err != nil {
		return err
	}
	if n >= background.MaxServices {
		return fmt.Errorf("a site may have at most %d background services", background.MaxServices)
	}
	if _, err := tx.Exec(`INSERT INTO bg_services (id, site_id, name, args, enabled, memory_mb, created_at) VALUES (?,?,?,?,?,?,?)`, id, siteID, in.Name, in.Args, in.Enabled, in.MemoryMB, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateBgService replaces the name, arguments, state and memory limit of a service. It must belong to the site.
func (s *Store) UpdateBgService(site string, kind sitepkg.Kind, id int64, in background.Input) error {
	if err := background.ValidService(kind, in); err != nil {
		return err
	}
	res, err := s.db.Exec(`UPDATE bg_services SET name = ?, args = ?, enabled = ?, memory_mb = ? WHERE id = ? AND site_id = (SELECT id FROM sites WHERE name = ?)`, in.Name, in.Args, in.Enabled, in.MemoryMB, id, site)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("site %q has no background service %d: %w", site, id, ErrNotFound)
	}
	return nil
}

// DeleteBgService removes a service. The service must belong to the site.
func (s *Store) DeleteBgService(site string, id int64) error {
	res, err := s.db.Exec(`DELETE FROM bg_services WHERE id = ? AND site_id = (SELECT id FROM sites WHERE name = ?)`, id, site)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("site %q has no background service %d: %w", site, id, ErrNotFound)
	}
	return nil
}
