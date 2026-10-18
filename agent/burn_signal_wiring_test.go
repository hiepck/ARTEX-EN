package agent

import (
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestGraphOverviewExposesBurnOnlyWhenEnabled(t *testing.T) {
	d := testDB(t)
	defer d.Close()

	task, err := d.CreateTaskWithOptions("burn wiring", "expose burn to the planner", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)

	store := d.Exploration(task.ExplorationID)
	intentID, err := store.AddIntent(map[string]any{"summary": "busy intent"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := store.ClaimIntent(intentID, "work#1"); err != nil || !ok {
		t.Fatalf("claim intent: ok=%v err=%v", ok, err)
	}
	if _, err := store.AppendActivity(db.Activity{NodeID: &intentID, Worker: "work#1", Kind: "tool_use"}); err != nil {
		t.Fatal(err)
	}

	tools := NewToolSet(store, "planner")

	old := BurnSignalEnabled
	t.Cleanup(func() { BurnSignalEnabled = old })

	BurnSignalEnabled = func() bool { return false }
	off := tools.graphOverviewData()
	for _, entry := range off["running_intents"].([]map[string]any) {
		if _, has := entry["burn"]; has {
			t.Fatalf("burn field present while BurnSignalEnabled is off: %+v", entry)
		}
	}

	BurnSignalEnabled = func() bool { return true }
	on := tools.graphOverviewData()
	running, _ := on["running_intents"].([]map[string]any)
	var found bool
	for _, entry := range running {
		if entry["id"] != intentID {
			continue
		}
		found = true
		burn, ok := entry["burn"].(map[string]any)
		if !ok {
			t.Fatalf("intent %d missing burn field while enabled: %+v", intentID, entry)
		}
		if tc, _ := burn["tool_calls"].(int); tc < 1 {
			t.Fatalf("burn.tool_calls = %v, want >= 1", burn["tool_calls"])
		}
	}
	if !found {
		t.Fatalf("intent %d not found in running_intents: %+v", intentID, running)
	}
}
