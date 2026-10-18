package server

import (
	"encoding/json"
	"testing"
)

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

// TestBurnSignalEnabledSettingsAPIRoundTrip confirms burn_signal_enabled is
// operator-reachable through the HTTP settings surface (GET /api/settings,
// PATCH via PUT /api/settings), the same way noa_compaction is, and that the
// HTTP layer and Manager.BurnSignalEnabled() agree afterward.
func TestBurnSignalEnabledSettingsAPIRoundTrip(t *testing.T) {
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.pg
	clearSetting := func() { _, _ = pg.Exec(`DELETE FROM settings WHERE key=$1`, settingBurnSignalEnabled) }
	clearSetting()
	t.Cleanup(clearSetting)

	readBurnSignal := func() bool {
		t.Helper()
		r := request("GET", "/api/settings", "")
		if r.Code != 200 {
			t.Fatalf("GET /api/settings failed %d: %s", r.Code, r.Body)
		}
		var payload map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode the settings payload: %v", err)
		}
		v, ok := payload["burn_signal_enabled"]
		if !ok {
			t.Fatal("settings payload is missing burn_signal_enabled")
		}
		on, ok := v.(bool)
		if !ok {
			t.Fatalf("burn_signal_enabled is not a bool: %T(%v)", v, v)
		}
		return on
	}

	if on := readBurnSignal(); on {
		t.Fatal("GET /api/settings burn_signal_enabled default = true, want false")
	}

	if r := request("PUT", "/api/settings", `{"burn_signal_enabled":true}`); r.Code != 200 {
		t.Fatalf("PUT /api/settings burn_signal_enabled failed %d: %s", r.Code, r.Body)
	}

	if on := readBurnSignal(); !on {
		t.Fatal("GET /api/settings burn_signal_enabled after PATCH(true) = false, want true")
	}
	if !s.m.BurnSignalEnabled() {
		t.Fatal("Manager.BurnSignalEnabled() after PATCH(true) = false, want true")
	}
}
