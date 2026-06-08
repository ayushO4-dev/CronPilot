package system

import "testing"

func TestCollect(t *testing.T) {
	s, err := Collect()
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if s.Time == 0 {
		t.Fatal("expected a non-zero timestamp")
	}
	if s.CPU.Cores <= 0 {
		t.Logf("warning: logical CPU count not detected on this platform")
	}
}

func TestSampler(t *testing.T) {
	sampler := NewSampler()
	if _, err := sampler.Sample(); err != nil {
		t.Fatalf("sample: %v", err)
	}
}
