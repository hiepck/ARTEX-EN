package server

import "testing"

func TestBurnSignalEnabledDefaultsOffAndPersists(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()

	if m.BurnSignalEnabled() {
		t.Fatal("BurnSignalEnabled() default = true, want false")
	}
	if err := m.SetBurnSignalEnabled(true); err != nil {
		t.Fatalf("SetBurnSignalEnabled: %v", err)
	}
	if !m.BurnSignalEnabled() {
		t.Fatal("BurnSignalEnabled() after SetBurnSignalEnabled(true) = false, want true")
	}
}
