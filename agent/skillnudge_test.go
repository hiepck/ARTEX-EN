package agent

import "testing"

func TestSkillCatalogDirNilSafe(t *testing.T) {
	old := SkillCatalogDir
	t.Cleanup(func() { SkillCatalogDir = old })
	SkillCatalogDir = nil
	if got := skillCatalogDir(); got != "" {
		t.Fatalf("skillCatalogDir() with nil var = %q, want \"\"", got)
	}
}

func TestSkillRelevanceNudgeEnabledNilSafe(t *testing.T) {
	old := SkillRelevanceNudgeEnabled
	t.Cleanup(func() { SkillRelevanceNudgeEnabled = old })
	SkillRelevanceNudgeEnabled = nil
	if skillRelevanceNudgeEnabled() {
		t.Fatal("skillRelevanceNudgeEnabled() with nil var = true, want false")
	}
}
