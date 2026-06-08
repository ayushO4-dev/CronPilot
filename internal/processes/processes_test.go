package processes

import (
	"context"
	"errors"
	"testing"
)

func TestSignalValidation(t *testing.T) {
	ctx := context.Background()
	// pid <= 1 is rejected before any kill is attempted.
	if err := Signal(ctx, 1, "TERM"); !errors.Is(err, ErrInvalidPID) {
		t.Errorf("pid=1: expected ErrInvalidPID, got %v", err)
	}
	if err := Signal(ctx, 0, "TERM"); !errors.Is(err, ErrInvalidPID) {
		t.Errorf("pid=0: expected ErrInvalidPID, got %v", err)
	}
	// Unknown signal is rejected before any kill is attempted.
	if err := Signal(ctx, 999999, "BOGUS"); !errors.Is(err, ErrInvalidSignal) {
		t.Errorf("bad signal: expected ErrInvalidSignal, got %v", err)
	}
}

func TestSamplerList(t *testing.T) {
	s := NewSampler()
	list, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected at least one process")
	}
	// A second call should succeed and exercise the delta path.
	if _, err := s.List(context.Background()); err != nil {
		t.Fatalf("second list: %v", err)
	}
}
