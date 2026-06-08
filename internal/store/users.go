package store

import (
	"database/sql"
	"errors"
	"time"
)

// User is an application account. Passwords are never stored in plaintext;
// PasswordHash holds an encoded Argon2id string.
type User struct {
	ID                 string
	Username           string
	PasswordHash       string
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// CountUsers returns the number of accounts; used to detect first-run.
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser inserts a new account.
func (s *Store) CreateUser(u *User) error {
	now := time.Now()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	_, err := s.db.Exec(
		`INSERT INTO users(id, username, password_hash, must_change_password, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, boolToInt(u.MustChangePassword), u.CreatedAt.Unix(), u.UpdatedAt.Unix(),
	)
	return err
}

// GetUserByUsername returns the account with the given username, or ErrNotFound.
func (s *Store) GetUserByUsername(username string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, password_hash, must_change_password, created_at, updated_at
		 FROM users WHERE username=?`, username))
}

// GetUserByID returns the account with the given id, or ErrNotFound.
func (s *Store) GetUserByID(id string) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, password_hash, must_change_password, created_at, updated_at
		 FROM users WHERE id=?`, id))
}

// UpdateUserPassword sets a new password hash and the must-change flag.
func (s *Store) UpdateUserPassword(id, hash string, mustChange bool) error {
	_, err := s.db.Exec(
		`UPDATE users SET password_hash=?, must_change_password=?, updated_at=? WHERE id=?`,
		hash, boolToInt(mustChange), time.Now().Unix(), id)
	return err
}

func (s *Store) scanUser(row *sql.Row) (*User, error) {
	var (
		u                User
		mustChange       int
		created, updated int64
	)
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &mustChange, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.MustChangePassword = mustChange != 0
	u.CreatedAt = time.Unix(created, 0)
	u.UpdatedAt = time.Unix(updated, 0)
	return &u, nil
}
