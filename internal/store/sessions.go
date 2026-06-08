package store

import (
	"database/sql"
	"errors"
	"time"
)

// Session is an opaque server-side login session keyed by a random token.
type Session struct {
	Token     string
	UserID    string
	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

// CreateSession persists a new session.
func (s *Store) CreateSession(sess *Session) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions(token, user_id, created_at, last_seen, expires_at)
		 VALUES(?, ?, ?, ?, ?)`,
		sess.Token, sess.UserID, sess.CreatedAt.Unix(), sess.LastSeen.Unix(), sess.ExpiresAt.Unix())
	return err
}

// GetSession looks up a session by token, or returns ErrNotFound.
func (s *Store) GetSession(token string) (*Session, error) {
	var (
		sess                       Session
		created, lastSeen, expires int64
	)
	err := s.db.QueryRow(
		`SELECT token, user_id, created_at, last_seen, expires_at FROM sessions WHERE token=?`, token).
		Scan(&sess.Token, &sess.UserID, &created, &lastSeen, &expires)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	sess.CreatedAt = time.Unix(created, 0)
	sess.LastSeen = time.Unix(lastSeen, 0)
	sess.ExpiresAt = time.Unix(expires, 0)
	return &sess, nil
}

// TouchSession updates the last-seen and expiry timestamps (idle-timeout slide).
func (s *Store) TouchSession(token string, lastSeen, expiresAt time.Time) error {
	_, err := s.db.Exec(`UPDATE sessions SET last_seen=?, expires_at=? WHERE token=?`,
		lastSeen.Unix(), expiresAt.Unix(), token)
	return err
}

// DeleteSession removes a single session (logout).
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token=?`, token)
	return err
}

// DeleteUserSessions removes all sessions for a user (e.g. after password change).
func (s *Store) DeleteUserSessions(userID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

// DeleteExpiredSessions purges sessions whose expiry is at or before now.
func (s *Store) DeleteExpiredSessions(now time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at<=?`, now.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
