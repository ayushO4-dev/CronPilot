package store

import "time"

// AuditEntry records a security-relevant event (login, privileged action, etc.).
type AuditEntry struct {
	ID       int64
	Time     time.Time
	UserID   string
	Username string
	Action   string
	Detail   string
	IP       string
}

// AddAudit appends an audit record. Time defaults to now when zero.
func (s *Store) AddAudit(e AuditEntry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO audit_log(ts, user_id, username, action, detail, ip) VALUES(?, ?, ?, ?, ?, ?)`,
		e.Time.Unix(), e.UserID, e.Username, e.Action, e.Detail, e.IP)
	return err
}

// ListAudit returns the most recent entries, newest first.
func (s *Store) ListAudit(limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(
		`SELECT id, ts, user_id, username, action, detail, ip
		 FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var (
			e  AuditEntry
			ts int64
		)
		if err := rows.Scan(&e.ID, &ts, &e.UserID, &e.Username, &e.Action, &e.Detail, &e.IP); err != nil {
			return nil, err
		}
		e.Time = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}
