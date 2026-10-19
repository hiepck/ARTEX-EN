package server

import (
	"encoding/json"
	"testing"
)

func TestSkillRelevanceNudgeEnabledDefaultsOffAndPersists(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	clearSetting := func() { _, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, "skill_relevance_nudge_enabled") }
	clearSetting()
	t.Cleanup(clearSetting)

	if m.SkillRelevanceNudgeEnabled() {
		t.Fatal("SkillRelevanceNudgeEnabled() default = true, want false")
	}
	if err := m.SetSkillRelevanceNudgeEnabled(true); err != nil {
		t.Fatalf("SetSkillRelevanceNudgeEnabled: %v", err)
	}
	if !m.SkillRelevanceNudgeEnabled() {
		t.Fatal("SkillRelevanceNudgeEnabled() after Set(true) = false, want true")
	}
}

func TestSkillRelevanceNudgeEnabledSettingsAPIRoundTrip(t *testing.T) {
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.pg
	clearSetting := func() { _, _ = pg.Exec(`DELETE FROM settings WHERE key=$1`, "skill_relevance_nudge_enabled") }
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
		v, ok := payload["skill_relevance_nudge_enabled"]
		if !ok {
			t.Fatal("settings payload is missing skill_relevance_nudge_enabled")
		}
		on, ok := v.(bool)
		if !ok {
			t.Fatalf("skill_relevance_nudge_enabled is not a bool: %T(%v)", v, v)
		}
		return on
	}

	if on := read(); on {
		t.Fatal("GET /api/settings skill_relevance_nudge_enabled default = true, want false")
	}
	if r := request("PUT", "/api/settings", `{"skill_relevance_nudge_enabled":true}`); r.Code != 200 {
		t.Fatalf("PUT /api/settings skill_relevance_nudge_enabled failed %d: %s", r.Code, r.Body)
	}
	if on := read(); !on {
		t.Fatal("GET /api/settings skill_relevance_nudge_enabled after PATCH(true) = false, want true")
	}
	if !s.m.SkillRelevanceNudgeEnabled() {
		t.Fatal("Manager.SkillRelevanceNudgeEnabled() after PATCH(true) = false, want true")
	}
}
