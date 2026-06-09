package store

import (
	"database/sql"
	"errors"
	"time"
)

// TaskRow is the persisted form of a task. The full ladder definition lives in
// Data as JSON; columns mirror the fields needed for listing and runtime state.
type TaskRow struct {
	ID         string
	Name       string
	Enabled    bool
	Data       string
	LastRun    *time.Time
	LastStatus string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SaveTask inserts or updates a task. last_run/last_status are left untouched.
func (s *Store) SaveTask(r *TaskRow) error {
	now := time.Now()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	_, err := s.db.Exec(
		`INSERT INTO tasks(id, name, enabled, data, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name=excluded.name, enabled=excluded.enabled, data=excluded.data, updated_at=excluded.updated_at`,
		r.ID, r.Name, boolToInt(r.Enabled), r.Data, r.CreatedAt.Unix(), r.UpdatedAt.Unix())
	return err
}

func (s *Store) scanTaskRow(scan func(dest ...any) error) (*TaskRow, error) {
	var (
		r                TaskRow
		enabled          int
		lastRun          sql.NullInt64
		lastStatus       sql.NullString
		created, updated int64
	)
	if err := scan(&r.ID, &r.Name, &enabled, &r.Data, &lastRun, &lastStatus, &created, &updated); err != nil {
		return nil, err
	}
	r.Enabled = enabled != 0
	if lastRun.Valid {
		t := time.Unix(lastRun.Int64, 0)
		r.LastRun = &t
	}
	r.LastStatus = lastStatus.String
	r.CreatedAt = time.Unix(created, 0)
	r.UpdatedAt = time.Unix(updated, 0)
	return &r, nil
}

const taskCols = `id, name, enabled, data, last_run, last_status, created_at, updated_at`

// GetTask returns a task row, or ErrNotFound.
func (s *Store) GetTask(id string) (*TaskRow, error) {
	row := s.db.QueryRow(`SELECT `+taskCols+` FROM tasks WHERE id=?`, id)
	r, err := s.scanTaskRow(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

// ListTasks returns all task rows ordered by name.
func (s *Store) ListTasks() ([]*TaskRow, error) {
	rows, err := s.db.Query(`SELECT ` + taskCols + ` FROM tasks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TaskRow
	for rows.Next() {
		r, err := s.scanTaskRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetTaskEnabled toggles a task's enabled flag.
func (s *Store) SetTaskEnabled(id string, enabled bool) error {
	_, err := s.db.Exec(`UPDATE tasks SET enabled=?, updated_at=? WHERE id=?`,
		boolToInt(enabled), time.Now().Unix(), id)
	return err
}

// UpdateTaskLastRun records the most recent run timestamp and status.
func (s *Store) UpdateTaskLastRun(id string, t time.Time, status string) error {
	_, err := s.db.Exec(`UPDATE tasks SET last_run=?, last_status=? WHERE id=?`, t.Unix(), status, id)
	return err
}

// DeleteTask removes a task (and its runs, via cascade).
func (s *Store) DeleteTask(id string) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}

// TaskRun is one recorded execution of a task.
type TaskRun struct {
	ID         int64     `json:"id"`
	TaskID     string    `json:"taskId"`
	Time       time.Time `json:"time"`
	Trigger    string    `json:"trigger"`
	OK         bool      `json:"ok"`
	Summary    string    `json:"summary"`
	Detail     string    `json:"detail,omitempty"`
	DurationMs int64     `json:"durationMs"`
}

// AddTaskRun appends a run record and sets r.ID.
func (s *Store) AddTaskRun(r *TaskRun) error {
	res, err := s.db.Exec(
		`INSERT INTO task_runs(task_id, ts, trigger, ok, summary, detail, duration_ms)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		r.TaskID, r.Time.Unix(), r.Trigger, boolToInt(r.OK), r.Summary, r.Detail, r.DurationMs)
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

// ListTaskRuns returns recent runs for a task, newest first.
func (s *Store) ListTaskRuns(taskID string, limit int) ([]TaskRun, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, task_id, ts, trigger, ok, summary, detail, duration_ms
		 FROM task_runs WHERE task_id=? ORDER BY id DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskRun
	for rows.Next() {
		var (
			r  TaskRun
			ts int64
			ok int
		)
		if err := rows.Scan(&r.ID, &r.TaskID, &ts, &r.Trigger, &ok, &r.Summary, &r.Detail, &r.DurationMs); err != nil {
			return nil, err
		}
		r.Time = time.Unix(ts, 0)
		r.OK = ok != 0
		out = append(out, r)
	}
	return out, rows.Err()
}
