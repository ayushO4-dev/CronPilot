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

// Rung is one line of the ladder: contacts (condition) -> actions. Each rung
// carries its own optional Trigger; a rung with no trigger runs only on demand
// ("run now") or when re-evaluated as part of a full-task scan.
type Rung struct {
	ID       string    `json:"id"`
	Label    string    `json:"label,omitempty"`
	Trigger  *Trigger  `json:"trigger,omitempty"`
	Match    MatchMode `json:"match"`
	Contacts []Contact `json:"contacts"`
	Actions  []Action  `json:"actions"`
}

// Task is a ladder program plus scheduling and runtime state.
type Task struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
	// LegacyTrigger captures a pre-per-rung, task-level trigger from older stored
	// JSON. Normalize migrates it onto rungs that lack one, then clears it; it is
	// never written back out.
	LegacyTrigger *Trigger   `json:"trigger,omitempty"`
	RunAs         string     `json:"runAs,omitempty"`
	Rungs         []Rung     `json:"rungs"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	LastRun       *time.Time `json:"lastRun,omitempty"`
	LastStatus    string     `json:"lastStatus,omitempty"`
}

var (
	contactKinds = map[string]bool{
		"service": true, "process": true, "time": true, "metric": true,
		"file": true, "flag": true, "taskState": true, "rung": true,
	}
	actionKinds = map[string]bool{
		"command": true, "service": true, "flag": true, "taskToggle": true, "log": true,
	}
)

// Normalize fills in missing IDs and defaults in place, and migrates a legacy
// task-level trigger onto rungs.
func (t *Task) Normalize() {
	if t.ID == "" {
		t.ID = newID("task")
	}
	for i := range t.Rungs {
		r := &t.Rungs[i]
		if r.ID == "" {
			r.ID = newID("rung")
		}
		if r.Match == "" {
			r.Match = MatchAll
		}
		// Migrate a legacy task-level schedule onto rungs that have none, so
		// tasks created before per-rung triggers keep firing.
		if r.Trigger == nil && t.LegacyTrigger != nil &&
			(t.LegacyTrigger.Type == TriggerInterval || t.LegacyTrigger.Type == TriggerCron) {
			tr := *t.LegacyTrigger
			r.Trigger = &tr
		}
		// A manual/empty rung trigger means "no schedule" — normalize to nil.
		if r.Trigger != nil && (r.Trigger.Type == "" || r.Trigger.Type == TriggerManual) {
			r.Trigger = nil
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
	t.LegacyTrigger = nil
}

// Validate checks structural correctness.
func (t *Task) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("name is required")
	}
	ids := make(map[string]bool, len(t.Rungs))
	for _, r := range t.Rungs {
		ids[r.ID] = true
	}
	for _, r := range t.Rungs {
		if r.Match != MatchAll && r.Match != MatchAny {
			return fmt.Errorf("rung %q: invalid match %q", r.ID, r.Match)
		}
		if r.Trigger != nil {
			switch r.Trigger.Type {
			case TriggerInterval:
				if r.Trigger.IntervalSeconds <= 0 {
					return fmt.Errorf("rung %q: interval trigger requires intervalSeconds > 0", r.ID)
				}
			case TriggerCron:
				if _, err := cron.ParseStandard(r.Trigger.Cron); err != nil {
					return fmt.Errorf("rung %q: invalid cron expression: %w", r.ID, err)
				}
			default:
				return fmt.Errorf("rung %q: invalid trigger type %q", r.ID, r.Trigger.Type)
			}
		}
		for _, c := range r.Contacts {
			if !contactKinds[c.Kind] {
				return fmt.Errorf("rung %q: invalid contact kind %q", r.ID, c.Kind)
			}
			if c.Kind == "rung" {
				ref, _ := c.Params["rung"].(string)
				if ref == "" || !ids[ref] {
					return fmt.Errorf("rung %q: 'rung' contact references unknown rung %q", r.ID, ref)
				}
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

// scheduleSpec returns the cron spec for this rung's trigger, or ("", false) if
// the rung has no schedule (it runs only on demand).
func (r *Rung) scheduleSpec() (string, bool) {
	if r.Trigger == nil {
		return "", false
	}
	switch r.Trigger.Type {
	case TriggerInterval:
		return "@every " + strconv.Itoa(r.Trigger.IntervalSeconds) + "s", true
	case TriggerCron:
		return r.Trigger.Cron, true
	default:
		return "", false
	}
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
