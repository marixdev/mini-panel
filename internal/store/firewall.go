package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mpn/internal/firewall"
)

// FirewallRules lists the rules in the order they were added.
func (s *Store) FirewallRules() ([]firewall.Rule, error) {
	rows, err := s.db.Query(`SELECT id, action, proto, port, source, comment FROM firewall_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []firewall.Rule{}
	for rows.Next() {
		var r firewall.Rule
		if err := rows.Scan(&r.ID, &r.Action, &r.Proto, &r.Port, &r.Source, &r.Comment); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddFirewallRule stores a rule after checking it. The same rule twice is refused, so a double click does nothing.
func (s *Store) AddFirewallRule(r firewall.Rule) (int64, error) {
	r, err := firewall.ValidRule(r)
	if err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM firewall_rules`).Scan(&n); err != nil {
		return 0, err
	}
	if n >= firewall.MaxRules {
		return 0, fmt.Errorf("at most %d firewall rules", firewall.MaxRules)
	}
	var same int
	if err := tx.QueryRow(`SELECT count(*) FROM firewall_rules WHERE action = ? AND proto = ? AND port = ? AND source = ?`, r.Action, r.Proto, r.Port, r.Source).Scan(&same); err != nil {
		return 0, err
	}
	if same > 0 {
		return 0, existsError{"firewall rule"}
	}
	res, err := tx.Exec(`INSERT INTO firewall_rules (action, proto, port, source, comment, created_at) VALUES (?,?,?,?,?,?)`, r.Action, r.Proto, r.Port, r.Source, r.Comment, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// DeleteFirewallRule removes a rule.
func (s *Store) DeleteFirewallRule(id int64) error {
	res, err := s.db.Exec(`DELETE FROM firewall_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("firewall rule %d: %w", id, ErrNotFound)
	}
	return nil
}

// Setting reads a server-wide setting; def is returned when it was never set.
func (s *Store) Setting(key, def string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return def, nil
	}
	return v, err
}

// SetSetting writes a server-wide setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

const firewallKey = "firewall"

// FirewallOn says whether the person turned the firewall on.
func (s *Store) FirewallOn() (bool, error) {
	v, err := s.Setting(firewallKey, "off")
	return v == "on", err
}

// SetFirewallOn records that the firewall is on or off.
func (s *Store) SetFirewallOn(on bool) error {
	if on {
		return s.SetSetting(firewallKey, "on")
	}
	return s.SetSetting(firewallKey, "off")
}
