package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/ayushkanoje/cronpilot/internal/store"
)

// Manager loads, schedules, evaluates and executes tasks.
type Manager struct {
	store *store.Store
	log   *slog.Logger

	mu            sync.Mutex       // guards tasks/flags/lastEnergized; held for a run
	flags         map[string]bool  // virtual flags set/read by actions/contacts
	tasks         map[string]*Task // in-memory cache for evaluation & scheduling
	lastEnergized map[string]bool  // "taskID/rungID" -> energized on last evaluation

	schedMu sync.Mutex // guards cron + entries
	cron    *cron.Cron
	entries map[string]cron.EntryID
}

// NewManager constructs a Manager.
func NewManager(st *store.Store, log *slog.Logger) *Manager {
	return &Manager{
		store:         st,
		log:           log,
		flags:         map[string]bool{},
		tasks:         map[string]*Task{},
		lastEnergized: map[string]bool{},
		entries:       map[string]cron.EntryID{},
	}
}

// Start loads tasks and begins the scheduler.
func (m *Manager) Start() error {
	rows, err := m.store.ListTasks()
	if err != nil {
		return err
	}
	m.mu.Lock()
	for _, row := range rows {
		if t, err := fromRow(row); err == nil {
			m.tasks[t.ID] = t
		} else {
			m.log.Error("load task", "id", row.ID, "err", err)
		}
	}
	m.mu.Unlock()

	m.schedMu.Lock()
	m.cron = cron.New()
	m.schedMu.Unlock()
	m.reschedule()
	m.cron.Start()
	m.log.Info("task engine started", "tasks", len(rows))
	return nil
}

// Stop halts the scheduler.
func (m *Manager) Stop() {
	m.schedMu.Lock()
	defer m.schedMu.Unlock()
	if m.cron != nil {
		m.cron.Stop()
	}
}

// reschedule rebuilds cron entries from the enabled, scheduled tasks.
func (m *Manager) reschedule() {
	m.mu.Lock()
	snap := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		snap = append(snap, t)
	}
	m.mu.Unlock()

	m.schedMu.Lock()
	defer m.schedMu.Unlock()
	if m.cron == nil {
		return
	}
	for _, id := range m.entries {
		m.cron.Remove(id)
	}
	m.entries = map[string]cron.EntryID{}
	for _, t := range snap {
		if !t.Enabled {
			continue
		}
		// Each rung with its own trigger schedules independently; rungs without
		// one run only on demand.
		for _, r := range t.Rungs {
			spec, ok := r.scheduleSpec()
			if !ok {
				continue
			}
			taskID, rungID := t.ID, r.ID
			key := taskID + "/" + rungID
			entryID, err := m.cron.AddFunc(spec, func() { m.runRung(taskID, rungID, "schedule") })
			if err != nil {
				m.log.Error("schedule rung", "task", taskID, "rung", rungID, "spec", spec, "err", err)
				continue
			}
			m.entries[key] = entryID
		}
	}
}

// List returns all tasks (persisted state).
func (m *Manager) List() ([]*Task, error) {
	rows, err := m.store.ListTasks()
	if err != nil {
		return nil, err
	}
	out := make([]*Task, 0, len(rows))
	for _, row := range rows {
		if t, err := fromRow(row); err == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

// Get returns one task.
func (m *Manager) Get(id string) (*Task, error) {
	row, err := m.store.GetTask(id)
	if err != nil {
		return nil, err
	}
	return fromRow(row)
}

// Create validates and persists a new task.
func (m *Manager) Create(t *Task) error {
	t.Normalize()
	if err := t.Validate(); err != nil {
		return err
	}
	t.CreatedAt = time.Now()
	row, err := m.toRow(t)
	if err != nil {
		return err
	}
	if err := m.store.SaveTask(row); err != nil {
		return err
	}
	t.UpdatedAt = row.UpdatedAt
	m.mu.Lock()
	m.tasks[t.ID] = t
	m.mu.Unlock()
	m.reschedule()
	return nil
}

// Update validates and persists changes to an existing task.
func (m *Manager) Update(t *Task) error {
	t.Normalize()
	if err := t.Validate(); err != nil {
		return err
	}
	if existing, err := m.store.GetTask(t.ID); err == nil {
		t.CreatedAt = existing.CreatedAt
		t.LastRun = existing.LastRun
		t.LastStatus = existing.LastStatus
	}
	row, err := m.toRow(t)
	if err != nil {
		return err
	}
	if err := m.store.SaveTask(row); err != nil {
		return err
	}
	m.mu.Lock()
	m.tasks[t.ID] = t
	m.mu.Unlock()
	m.reschedule()
	return nil
}

// Delete removes a task.
func (m *Manager) Delete(id string) error {
	if err := m.store.DeleteTask(id); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.tasks, id)
	m.mu.Unlock()
	m.reschedule()
	return nil
}

// SetEnabled toggles a task's enabled flag and reschedules.
func (m *Manager) SetEnabled(id string, enabled bool) error {
	if err := m.store.SetTaskEnabled(id, enabled); err != nil {
		return err
	}
	m.mu.Lock()
	if t := m.tasks[id]; t != nil {
		t.Enabled = enabled
	}
	m.mu.Unlock()
	m.reschedule()
	return nil
}

// RunNow evaluates and executes a task immediately.
func (m *Manager) RunNow(id string) (*store.TaskRun, error) {
	m.mu.Lock()
	_, ok := m.tasks[id]
	m.mu.Unlock()
	if !ok {
		t, err := m.Get(id)
		if err != nil {
			return nil, err
		}
		m.mu.Lock()
		m.tasks[id] = t
		m.mu.Unlock()
	}
	return m.runTask(id, "manual"), nil
}

// Runs returns recent run history for a task.
func (m *Manager) Runs(id string, limit int) ([]store.TaskRun, error) {
	return m.store.ListTaskRuns(id, limit)
}

// runTask evaluates every rung and executes the actions of energized rungs. Used
// by "run now" (a full top-to-bottom scan, regardless of per-rung triggers).
func (m *Manager) runTask(id, trigger string) *store.TaskRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[id]
	if t == nil {
		return nil
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ec := m.newEvalContext(ctx, id)

	var results []ActionResult
	fired := 0
	ok := true
	// Compute every rung's energized state against the *previous* values, then
	// commit together, so "rung" contacts observe the last evaluation (not this
	// pass's earlier rungs).
	newEnergized := make(map[string]bool, len(t.Rungs))
	for _, r := range t.Rungs {
		en := ec.rungEnergized(r)
		newEnergized[id+"/"+r.ID] = en
		if !en {
			continue
		}
		fired++
		for _, a := range r.Actions {
			res := m.execAction(ctx, a, t.RunAs)
			if !res.OK {
				ok = false
			}
			results = append(results, res)
		}
	}
	for k, v := range newEnergized {
		m.lastEnergized[k] = v
	}

	status := "ok"
	if !ok {
		status = "error"
	}
	summary := fmt.Sprintf("%d rung(s) energized, %d action(s)", fired, len(results))
	if !ok {
		summary += " — with errors"
	}
	m.log.Info("task run", "task", t.Name, "trigger", trigger, "fired", fired, "ok", ok)
	return m.recordRun(t, start, trigger, status, summary, ok, results)
}

// runRung evaluates a single rung (its own trigger fired) and runs its actions
// if energized. Records the rung's energized state and one run-history entry.
func (m *Manager) runRung(taskID, rungID, trigger string) *store.TaskRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[taskID]
	if t == nil {
		return nil
	}
	var rp *Rung
	for i := range t.Rungs {
		if t.Rungs[i].ID == rungID {
			rp = &t.Rungs[i]
			break
		}
	}
	if rp == nil {
		return nil
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	ec := m.newEvalContext(ctx, taskID)

	energized := ec.rungEnergized(*rp)
	var results []ActionResult
	ok := true
	if energized {
		for _, a := range rp.Actions {
			res := m.execAction(ctx, a, t.RunAs)
			if !res.OK {
				ok = false
			}
			results = append(results, res)
		}
	}
	m.lastEnergized[taskID+"/"+rungID] = energized // commit after evaluation

	status := "ok"
	if !ok {
		status = "error"
	}
	var summary string
	if !energized {
		summary = fmt.Sprintf("rung %s not energized", rungLabel(rp))
	} else {
		summary = fmt.Sprintf("rung %s energized, %d action(s)", rungLabel(rp), len(results))
		if !ok {
			summary += " — with errors"
		}
	}
	m.log.Info("rung run", "task", t.Name, "rung", rungID, "trigger", trigger, "energized", energized, "ok", ok)
	return m.recordRun(t, start, trigger, status, summary, ok, results)
}

// recordRun persists a run-history entry and updates the task's last-run state.
// Assumes m.mu is held.
func (m *Manager) recordRun(t *Task, start time.Time, trigger, status, summary string, ok bool, results []ActionResult) *store.TaskRun {
	detail, _ := json.Marshal(results)
	run := &store.TaskRun{
		TaskID:     t.ID,
		Time:       start,
		Trigger:    trigger,
		OK:         ok,
		Summary:    summary,
		Detail:     string(detail),
		DurationMs: time.Since(start).Milliseconds(),
	}
	if err := m.store.AddTaskRun(run); err != nil {
		m.log.Error("record task run", "task", t.ID, "err", err)
	}
	if err := m.store.UpdateTaskLastRun(t.ID, start, status); err != nil {
		m.log.Error("update task last run", "task", t.ID, "err", err)
	}
	t.LastRun = &start
	t.LastStatus = status
	return run
}

func rungLabel(r *Rung) string {
	if r.Label != "" {
		return r.Label
	}
	return r.ID
}

func (m *Manager) toRow(t *Task) (*store.TaskRow, error) {
	clone := *t
	clone.LastRun = nil
	clone.LastStatus = ""
	data, err := json.Marshal(&clone)
	if err != nil {
		return nil, err
	}
	return &store.TaskRow{
		ID:        t.ID,
		Name:      t.Name,
		Enabled:   t.Enabled,
		Data:      string(data),
		CreatedAt: t.CreatedAt,
	}, nil
}

func fromRow(row *store.TaskRow) (*Task, error) {
	var t Task
	if err := json.Unmarshal([]byte(row.Data), &t); err != nil {
		return nil, err
	}
	t.ID = row.ID
	t.Name = row.Name
	t.Enabled = row.Enabled
	t.LastRun = row.LastRun
	t.LastStatus = row.LastStatus
	t.CreatedAt = row.CreatedAt
	t.UpdatedAt = row.UpdatedAt
	return &t, nil
}
