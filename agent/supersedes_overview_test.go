package agent

import (
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestGraphOverviewExposesFactSupersedesWithOldIntentID(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes overview", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)

	intentID, err := store.AddIntent(map[string]any{"summary": "probe getFollowerAssets"}, 5, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	oldFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "403, looks like a seller-state gate"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Link(intentID, db.RelYields, oldFactID); err != nil {
		t.Fatal(err)
	}
	// Simulate the intent having long since finished and scrolled out of any
	// recent-window list -- the overview must still resolve old_intent_id
	// correctly, since it is computed from factFrom, not from recent_done_intents.
	if err := store.SetIntentState(intentID, "done"); err != nil {
		t.Fatal(err)
	}

	tools := NewToolSet(store, "planner")
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })

	FactSupersedesEnabled = func() bool { return false }
	off := tools.graphOverviewData()
	if _, has := off["fact_supersedes"]; has {
		t.Fatalf("fact_supersedes present while disabled: %+v", off["fact_supersedes"])
	}

	FactSupersedesEnabled = func() bool { return true }
	onEmpty := tools.graphOverviewData()
	if sup, ok := onEmpty["fact_supersedes"].([]map[string]any); !ok || len(sup) != 0 {
		t.Fatalf("fact_supersedes with no edges yet = %#v, want an empty slice, not absent/nil", onEmpty["fact_supersedes"])
	}

	newFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "fresh session token works, it was a credential-type issue"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Link(newFactID, db.RelSupersedes, oldFactID); err != nil {
		t.Fatal(err)
	}

	on := tools.graphOverviewData()
	sup, ok := on["fact_supersedes"].([]map[string]any)
	if !ok || len(sup) != 1 {
		t.Fatalf("fact_supersedes = %#v, want exactly 1 entry", on["fact_supersedes"])
	}
	if sup[0]["new_fact_id"] != newFactID || sup[0]["old_fact_id"] != oldFactID {
		t.Fatalf("entry = %+v, want new=%d old=%d", sup[0], newFactID, oldFactID)
	}
	if sup[0]["old_intent_id"] != intentID {
		t.Fatalf("old_intent_id = %v, want %d (must resolve via factFrom, not a windowed intent list)", sup[0]["old_intent_id"], intentID)
	}
}
