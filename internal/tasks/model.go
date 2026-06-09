// Package tasks implements CronPilot's ladder-logic automation engine.
//
// A Task is a small ladder program: a list of rungs. Each Rung is a boolean
// condition over "contacts" (ALL = series/AND, ANY = parallel/OR, with optional
// per-contact negation) that, when true at evaluation time, energizes its
// "actions" (coils). Tasks are evaluated on a schedule (interval or cron) or on
// demand ("run now").
package tasks

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// TriggerType selects how a task is evaluated.
type TriggerType string

const (
	TriggerInterval TriggerType = "interval"
	TriggerCron     TriggerType = "cron"
	TriggerManual   TriggerType = "manual"
)

// MatchMode is how a rung combines its contacts.
type MatchMode string

const (
	MatchAll MatchMode = "all" // series / AND
	MatchAny MatchMode = "any" // parallel / OR
)

// Trigger describes when a task runs.
type Trigger struct {
	Type            TriggerType `json:"type"`
	IntervalSeconds int         `json:"intervalSeconds,omitempty"`
	Cron            string      `json:"cron,omitempty"`
}

// Contact is one boolean input. Params are kind-specific.
type Contact struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"` // service|process|time|metric|file|flag|taskState
	Negate bool           `json:"negate,omitempty"`
	Params map[string]any `json:"params"`
}

// Action is one output ("coil"). Params are kind-specific.
type Action struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"` // command|service|flag|taskToggle|log
	Params map[string]any `json:"params"`
}

// Rung is one line of the ladder: contacts (condition) -> actions.
type Rung struct {
	ID       string    `json:"id"`
	Label    string    `json:"label,omitempty"`
	Match    MatchMode `json:"match"`
	Contacts []Contact `json:"contacts"`
	Actions  []Action  `json:"actions"`
}

// Task is a ladder program plus scheduling and runtime state.
type Task struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Enabled     bool       `json:"enabled"`
	Trigger     Trigger    `json:"trigger"`
	RunAs       string     `json:"runAs,omitempty"`
	Rungs       []Rung     `json:"rungs"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	LastRun     *time.Time `json:"lastRun,omitempty"`
	LastStatus  string     `json:"lastStatus,omitempty"`
}

var (
	contactKinds = map[string]bool{
		"service": true, "process": true, "time": true, "metric": true,
		"file": true, "flag": true, "taskState": true,
	}
	actionKinds = map[string]bool{
		"command": true, "service": true, "flag": true, "taskToggle": true, "log": true,
	}
)

// Normalize fills in missing IDs and defaults in place.
func (t *Task) Normalize() {
	if t.ID == "" {
		t.ID = newID("task")
	}
	if t.Trigger.Type == "" {
		t.Trigger.Type = TriggerManual
	}
	for i := range t.Rungs {
		r := &t.Rungs[i]
		if r.ID == "" {
			r.ID = newID("rung")
		}
		if r.Match == "" {
			r.Match = MatchAll
		}
		for j := range r.Contacts {
			if r.Contacts[j].ID == "" {
				r.Contacts[j].ID = newID("c")
			}
			if r.Contacts[j].Params == nil {
				r.Contacts[j].Params = map[string]any{}
			}
		}
		for j := range r.Actions {
			if r.Actions[j].ID == "" {
				r.Actions[j].ID = newID("a")
			}
			if r.Actions[j].Params == nil {
				r.Actions[j].Params = map[string]any{}
			}
		}
	}
}

// Validate checks structural correctness.
func (t *Task) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("name is required")
	}
	switch t.Trigger.Type {
	case TriggerInterval:
		if t.Trigger.IntervalSeconds <= 0 {
			return errors.New("interval trigger requires intervalSeconds > 0")
		}
	case TriggerCron:
		if _, err := cron.ParseStandard(t.Trigger.Cron); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	case TriggerManual:
		// no schedule
	default:
		return fmt.Errorf("invalid trigger type %q", t.Trigger.Type)
	}
	for _, r := range t.Rungs {
		if r.Match != MatchAll && r.Match != MatchAny {
			return fmt.Errorf("rung %q: invalid match %q", r.ID, r.Match)
		}
		for _, c := range r.Contacts {
			if !contactKinds[c.Kind] {
				return fmt.Errorf("rung %q: invalid contact kind %q", r.ID, c.Kind)
			}
		}
		if len(r.Actions) == 0 {
			return fmt.Errorf("rung %q: at least one action is required", r.ID)
		}
		for _, a := range r.Actions {
			if !actionKinds[a.Kind] {
				return fmt.Errorf("rung %q: invalid action kind %q", r.ID, a.Kind)
			}
		}
	}
	return nil
}

// scheduleSpec returns the cron spec for this task, or ("", false) if it is
// manual-only.
func (t *Task) scheduleSpec() (string, bool) {
	switch t.Trigger.Type {
	case TriggerInterval:
		return "@every " + strconv.Itoa(t.Trigger.IntervalSeconds) + "s", true
	case TriggerCron:
		return t.Trigger.Cron, true
	default:
		return "", false
	}
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
