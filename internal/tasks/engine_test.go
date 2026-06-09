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
	ec := m.newEvalContext(context.Background())

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
		Name:    "ok",
		Trigger: Trigger{Type: TriggerManual},
		Rungs:   []Rung{rung(MatchAll, flagContact("a", false))},
	}
	ok.Normalize()
	if err := ok.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}

	noName := &Task{Trigger: Trigger{Type: TriggerManual}}
	noName.Normalize()
	if err := noName.Validate(); err == nil {
		t.Error("expected error for missing name")
	}

	badCron := &Task{Name: "x", Trigger: Trigger{Type: TriggerCron, Cron: "not a cron"}}
	badCron.Normalize()
	if err := badCron.Validate(); err == nil {
		t.Error("expected error for invalid cron")
	}

	noAction := &Task{
		Name:    "x",
		Trigger: Trigger{Type: TriggerManual},
		Rungs:   []Rung{{Match: MatchAll, Contacts: []Contact{flagContact("a", false)}}},
	}
	noAction.Normalize()
	if err := noAction.Validate(); err == nil {
		t.Error("expected error for rung without actions")
	}
}

func TestNormalizeAssignsIDs(t *testing.T) {
	tk := &Task{
		Name:    "x",
		Trigger: Trigger{Type: TriggerManual},
		Rungs:   []Rung{rung(MatchAll, flagContact("a", false))},
	}
	tk.Normalize()
	if tk.ID == "" || tk.Rungs[0].ID == "" || tk.Rungs[0].Contacts[0].ID == "" || tk.Rungs[0].Actions[0].ID == "" {
		t.Error("expected IDs to be assigned")
	}
}
