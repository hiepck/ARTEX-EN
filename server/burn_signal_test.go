package server

import "testing"

func TestBurnSignalEnabledDefaultsOffAndPersists(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	// burn_signal_enabled lives in the shared `settings` table, not anything
	// per-test-run-scoped: a prior run (manual or automated) against this same
	// Postgres instance may have already set it to true and left it that way.
	// Delete the row outright so "default" below genuinely exercises the
	// unset -> GetBool's def=false path, and clean up afterward so this test
	// leaves no residue for the next run either.
	clearSetting := func() { _, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, "burn_signal_enabled") }
	clearSetting()
	t.Cleanup(clearSetting)

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
