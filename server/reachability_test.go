package server

import (
	"encoding/json"
	"testing"
)

func TestReachabilityEdgesEnabledDefaultsOffAndPersists(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	clearSetting := func() { _, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, "reachability_edges_enabled") }
	clearSetting()
	t.Cleanup(clearSetting)

	if m.ReachabilityEdgesEnabled() {
		t.Fatal("ReachabilityEdgesEnabled() default = true, want false")
	}
	if err := m.SetReachabilityEdgesEnabled(true); err != nil {
		t.Fatalf("SetReachabilityEdgesEnabled: %v", err)
	}
	if !m.ReachabilityEdgesEnabled() {
		t.Fatal("ReachabilityEdgesEnabled() after Set(true) = false, want true")
	}
}

// TestReachabilityEdgesEnabledSettingsAPIRoundTrip confirms
// reachability_edges_enabled is operator-reachable through the HTTP settings
// surface, the same way burn_signal_enabled already is.
func TestReachabilityEdgesEnabledSettingsAPIRoundTrip(t *testing.T) {
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.pg
	clearSetting := func() { _, _ = pg.Exec(`DELETE FROM settings WHERE key=$1`, settingReachabilityEdges) }
	clearSetting()
	t.Cleanup(clearSetting)

	readReachability := func() bool {
		t.Helper()
		r := request("GET", "/api/settings", "")
		if r.Code != 200 {
			t.Fatalf("GET /api/settings failed %d: %s", r.Code, r.Body)
		}
		var payload map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode the settings payload: %v", err)
		}
		v, ok := payload["reachability_edges_enabled"]
		if !ok {
			t.Fatal("settings payload is missing reachability_edges_enabled")
		}
		on, ok := v.(bool)
		if !ok {
			t.Fatalf("reachability_edges_enabled is not a bool: %T(%v)", v, v)
		}
		return on
	}

	if on := readReachability(); on {
		t.Fatal("GET /api/settings reachability_edges_enabled default = true, want false")
	}

	if r := request("PUT", "/api/settings", `{"reachability_edges_enabled":true}`); r.Code != 200 {
		t.Fatalf("PUT /api/settings reachability_edges_enabled failed %d: %s", r.Code, r.Body)
	}

	if on := readReachability(); !on {
		t.Fatal("GET /api/settings reachability_edges_enabled after PATCH(true) = false, want true")
	}
	if !s.m.ReachabilityEdgesEnabled() {
		t.Fatal("Manager.ReachabilityEdgesEnabled() after PATCH(true) = false, want true")
	}
}
