package agent

import (
	"testing"
)

func TestGraphOverviewGoalsIncludeVulnclass(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("vulnclass fixture", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	if _, err := store.AddGoal(map[string]any{"text": "find access control bugs", "vulnclass": "IDOR"}, "human"); err != nil {
		t.Fatal(err)
	}

	tools := NewToolSet(store, "planner")
	out := tools.graphOverviewData()
	goals, ok := out["goals"].([]map[string]any)
	if !ok {
		t.Fatalf("out[\"goals\"] = %#v, want []map[string]any", out["goals"])
	}
	var found bool
	for _, g := range goals {
		if g["vulnclass"] == "IDOR" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no goal entry carries vulnclass=IDOR in %+v", goals)
	}
}
