package server

import (
	"encoding/json"
	"testing"
)

func TestRootCauseGroupingEnabledDefaultsOffAndPersists(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	clearSetting := func() { _, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, "root_cause_grouping_enabled") }
	clearSetting()
	t.Cleanup(clearSetting)

	if m.RootCauseGroupingEnabled() {
		t.Fatal("RootCauseGroupingEnabled() default = true, want false")
	}
	if err := m.SetRootCauseGroupingEnabled(true); err != nil {
		t.Fatalf("SetRootCauseGroupingEnabled: %v", err)
	}
	if !m.RootCauseGroupingEnabled() {
		t.Fatal("RootCauseGroupingEnabled() after Set(true) = false, want true")
	}
}

func TestRootCauseGroupingEnabledSettingsAPIRoundTrip(t *testing.T) {
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.pg
	clearSetting := func() { _, _ = pg.Exec(`DELETE FROM settings WHERE key=$1`, "root_cause_grouping_enabled") }
	clearSetting()
	t.Cleanup(clearSetting)

	read := func() bool {
		t.Helper()
		r := request("GET", "/api/settings", "")
		if r.Code != 200 {
			t.Fatalf("GET /api/settings failed %d: %s", r.Code, r.Body)
		}
		var payload map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode the settings payload: %v", err)
		}
		v, ok := payload["root_cause_grouping_enabled"]
		if !ok {
			t.Fatal("settings payload is missing root_cause_grouping_enabled")
		}
		on, ok := v.(bool)
		if !ok {
			t.Fatalf("root_cause_grouping_enabled is not a bool: %T(%v)", v, v)
		}
		return on
	}

	if on := read(); on {
		t.Fatal("GET /api/settings root_cause_grouping_enabled default = true, want false")
	}
	if r := request("PUT", "/api/settings", `{"root_cause_grouping_enabled":true}`); r.Code != 200 {
		t.Fatalf("PUT /api/settings root_cause_grouping_enabled failed %d: %s", r.Code, r.Body)
	}
	if on := read(); !on {
		t.Fatal("GET /api/settings root_cause_grouping_enabled after PATCH(true) = false, want true")
	}
	if !s.m.RootCauseGroupingEnabled() {
		t.Fatal("Manager.RootCauseGroupingEnabled() after PATCH(true) = false, want true")
	}
}
