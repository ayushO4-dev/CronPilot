package tasks

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

func testManager() *Manager {
	return NewManager(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func rung(match MatchMode, contacts ...Contact) Rung {
	return Rung{Match: match, Contacts: contacts, Actions: []Action{{Kind: "log"}}}
}

func flagContact(name string, negate bool) Contact {
	return Contact{Kind: "flag", Negate: negate, Params: map[string]any{"name": name}}
}

func TestRungEnergized(t *testing.T) {
	m := testManager()
	m.flags["a"] = true
	m.flags["b"] = false
	ec := m.newEvalContext(context.Background(), "t")

	cases := []struct {
		name string
		rung Rung
		want bool
	}{
		{"empty is always energized", rung(MatchAll), true},
		{"all true", rung(MatchAll, flagContact("a", false)), true},
		{"all with one false", rung(MatchAll, flagContact("a", false), flagContact("b", false)), false},
		{"any with one true", rung(MatchAny, flagContact("a", false), flagContact("b", false)), true},
		{"any all false", rung(MatchAny, flagContact("b", false)), false},
		{"negate flips", rung(MatchAll, flagContact("b", true)), true},
	}
	for _, c := range cases {
		if got := ec.rungEnergized(c.rung); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := &Task{
		Name:  "ok",
		Rungs: []Rung{rung(MatchAll, flagContact("a", false))},
	}
	ok.Normalize()
	if err := ok.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	noName := &Task{}
	noName.Normalize()
	if err := noName.Validate(); err == nil {
		t.Error("expected error for missing name")
	}

	badCron := &Task{Name: "x", Rungs: []Rung{{
		Match:   MatchAll,
		Trigger: &Trigger{Type: TriggerCron, Cron: "not a cron"},
		Actions: []Action{{Kind: "log"}},
	}}}
	badCron.Normalize()
	if err := badCron.Validate(); err == nil {
		t.Error("expected error for invalid rung cron")
	}

	badInterval := &Task{Name: "x", Rungs: []Rung{{
		Match:   MatchAll,
		Trigger: &Trigger{Type: TriggerInterval, IntervalSeconds: 0},
		Actions: []Action{{Kind: "log"}},
	}}}
	badInterval.Normalize()
	if err := badInterval.Validate(); err == nil {
		t.Error("expected error for non-positive interval")
	}

	noAction := &Task{
		Name:  "x",
		Rungs: []Rung{{Match: MatchAll, Contacts: []Contact{flagContact("a", false)}}},
	}
	noAction.Normalize()
	if err := noAction.Validate(); err == nil {
		t.Error("expected error for rung without actions")
	}

	badRungRef := &Task{Name: "x", Rungs: []Rung{{
		Match:    MatchAll,
		Contacts: []Contact{{Kind: "rung", Params: map[string]any{"rung": "nope"}}},
		Actions:  []Action{{Kind: "log"}},
	}}}
	badRungRef.Normalize()
	if err := badRungRef.Validate(); err == nil {
		t.Error("expected error for unknown rung reference")
	}
}

func TestNormalizeAssignsIDs(t *testing.T) {
	tk := &Task{
		Name:  "x",
		Rungs: []Rung{rung(MatchAll, flagContact("a", false))},
	}
	tk.Normalize()
	if tk.ID == "" || tk.Rungs[0].ID == "" || tk.Rungs[0].Contacts[0].ID == "" || tk.Rungs[0].Actions[0].ID == "" {
		t.Error("expected IDs to be assigned")
	}
}

func TestMigrateLegacyTrigger(t *testing.T) {
	withCron := rung(MatchAll)
	withCron.Trigger = &Trigger{Type: TriggerCron, Cron: "* * * * *"}
	tk := &Task{
		Name:          "x",
		LegacyTrigger: &Trigger{Type: TriggerInterval, IntervalSeconds: 30},
		Rungs: []Rung{
			rung(MatchAll, flagContact("a", false)), // no trigger -> inherits legacy
			withCron,                                // keeps its own
		},
	}
	tk.Normalize()
	if tk.LegacyTrigger != nil {
		t.Error("expected legacy trigger cleared after migration")
	}
	if tk.Rungs[0].Trigger == nil || tk.Rungs[0].Trigger.Type != TriggerInterval {
		t.Errorf("expected rung 0 to inherit interval trigger, got %+v", tk.Rungs[0].Trigger)
	}
	if tk.Rungs[1].Trigger == nil || tk.Rungs[1].Trigger.Type != TriggerCron {
		t.Error("expected rung 1 to keep its own cron trigger")
	}
}

func TestRungContact(t *testing.T) {
	m := testManager()
	m.lastEnergized["task1/rungA"] = true
	ec := m.newEvalContext(context.Background(), "task1")
	if !ec.contactTrue(Contact{Kind: "rung", Params: map[string]any{"rung": "rungA"}}) {
		t.Error("expected energized rungA -> true")
	}
	if ec.contactTrue(Contact{Kind: "rung", Negate: true, Params: map[string]any{"rung": "rungA"}}) {
		t.Error("expected negated rungA -> false")
	}
	if ec.contactTrue(Contact{Kind: "rung", Params: map[string]any{"rung": "rungB"}}) {
		t.Error("expected unknown rung -> false")
	}
}
