package db

import "testing"

func TestInvokedSkillsFiltersToFoundRowsOfThisExploration(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()

	expID, err := d.CreateExploration("test", "invoked skills fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM explorations WHERE id=$1`, expID)
	otherExpID, err := d.CreateExploration("test", "a different exploration")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM explorations WHERE id=$1`, otherExpID)
	es := d.Exploration(expID)

	if err := d.InsertSkillUsage(&SkillUsage{Skill: "hunt-idor", AgentKey: "worker", ExplorationID: expID, Found: true}); err != nil {
		t.Fatal(err)
	}
	if err := d.InsertSkillUsage(&SkillUsage{Skill: "nonexistent-skill", AgentKey: "worker", ExplorationID: expID, Found: false}); err != nil {
		t.Fatal(err)
	}
	if err := d.InsertSkillUsage(&SkillUsage{Skill: "hunt-ssrf", AgentKey: "worker", ExplorationID: otherExpID, Found: true}); err != nil {
		t.Fatal(err)
	}

	invoked, err := es.InvokedSkills()
	if err != nil {
		t.Fatal(err)
	}
	if !invoked["hunt-idor"] {
		t.Fatalf("invoked = %v, want hunt-idor present (a real HIT in this exploration)", invoked)
	}
	if invoked["nonexistent-skill"] {
		t.Fatalf("invoked = %v, want nonexistent-skill absent (found=false is a MISS, not an invocation)", invoked)
	}
	if invoked["hunt-ssrf"] {
		t.Fatalf("invoked = %v, want hunt-ssrf absent (belongs to a different exploration)", invoked)
	}
}
