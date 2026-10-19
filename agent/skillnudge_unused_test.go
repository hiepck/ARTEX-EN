package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func writeFixtureSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGraphOverviewExposesUnusedRelevantSkillsAndSuppressesInvoked(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("skill nudge fixture", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	if _, err := store.AddGoal(map[string]any{"text": "look for coupon discount abuse in the checkout flow"}, "human"); err != nil {
		t.Fatal(err)
	}

	catalog := t.TempDir()
	writeFixtureSkill(t, catalog, "hunt-business-logic", "Covers coupon and discount abuse, price manipulation, and checkout logic flaws. Use when hunting business logic bugs.")
	writeFixtureSkill(t, catalog, "hunt-unrelated", "Covers only discount codes in an unrelated payment gateway context with nothing else in common.")

	oldEnabled, oldDir := SkillRelevanceNudgeEnabled, SkillCatalogDir
	t.Cleanup(func() { SkillRelevanceNudgeEnabled, SkillCatalogDir = oldEnabled, oldDir })
	SkillCatalogDir = func() string { return catalog }

	tools := NewToolSet(store, "planner")

	SkillRelevanceNudgeEnabled = func() bool { return false }
	off := tools.graphOverviewData()
	if _, has := off["unused_relevant_skills"]; has {
		t.Fatalf("unused_relevant_skills present while disabled: %+v", off["unused_relevant_skills"])
	}

	SkillRelevanceNudgeEnabled = func() bool { return true }
	on := tools.graphOverviewData()
	sug, ok := on["unused_relevant_skills"].([]map[string]any)
	if !ok {
		t.Fatalf("unused_relevant_skills = %#v, want []map[string]any", on["unused_relevant_skills"])
	}
	var names []string
	for _, s := range sug {
		names = append(names, s["skill"].(string))
	}
	if !contains(names, "hunt-business-logic") {
		t.Fatalf("suggestions = %v, want hunt-business-logic present (shares coupon+discount+checkout, >= 2 tokens)", names)
	}
	if contains(names, "hunt-unrelated") {
		t.Fatalf("suggestions = %v, want hunt-unrelated absent (shares only 1 token: discount)", names)
	}

	// Simulate the skill having been invoked: it must disappear next round.
	if err := d.InsertSkillUsage(&db.SkillUsage{Skill: "hunt-business-logic", AgentKey: "worker", ExplorationID: task.ExplorationID, Found: true}); err != nil {
		t.Fatal(err)
	}
	after := tools.graphOverviewData()
	sug2 := after["unused_relevant_skills"].([]map[string]any)
	for _, s := range sug2 {
		if s["skill"] == "hunt-business-logic" {
			t.Fatalf("hunt-business-logic still suggested after being invoked: %+v", sug2)
		}
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestGraphOverviewSkillMatchCanComeFromVulnclassAlone pins Review Focus's
// 4th item: a goal's free-text "text" field shares nothing with the skill
// below, but its "vulnclass" tag alone supplies 2 shared tokens -- proving
// vulnclass actually participates in matching, not just gsum's shape.
func TestGraphOverviewSkillMatchCanComeFromVulnclassAlone(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("vulnclass-only match fixture", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	if _, err := store.AddGoal(map[string]any{
		"text":      "examine the payment integration thoroughly",
		"vulnclass": "ssrf webhook",
	}, "human"); err != nil {
		t.Fatal(err)
	}

	catalog := t.TempDir()
	writeFixtureSkill(t, catalog, "hunt-ssrf", "Covers SSRF via webhook url fields and image upload endpoints.")

	oldEnabled, oldDir := SkillRelevanceNudgeEnabled, SkillCatalogDir
	t.Cleanup(func() { SkillRelevanceNudgeEnabled, SkillCatalogDir = oldEnabled, oldDir })
	SkillCatalogDir = func() string { return catalog }
	SkillRelevanceNudgeEnabled = func() bool { return true }

	tools := NewToolSet(store, "planner")
	out := tools.graphOverviewData()
	sug, ok := out["unused_relevant_skills"].([]map[string]any)
	if !ok || len(sug) != 1 || sug[0]["skill"] != "hunt-ssrf" {
		t.Fatalf("unused_relevant_skills = %#v, want exactly [hunt-ssrf] (matched via vulnclass tokens ssrf+webhook, free text shares nothing)", out["unused_relevant_skills"])
	}
}
