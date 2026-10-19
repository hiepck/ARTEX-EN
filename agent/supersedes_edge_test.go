package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestRecordFactSupersedesRequiresEvidence(t *testing.T) {
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })
	FactSupersedesEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes no evidence", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	oldFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "old blocking fact"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"summary": "new fact", "confidence": "inferred", "evidence": "",
		"supersedes": []int64{oldFactID},
	})
	res, err := ts.recordFact().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "observed") && !strings.Contains(res.Flatten(), "evidence") {
		t.Fatalf("expected an evidence-required rejection, got: %s", res.Flatten())
	}
	facts, _ := store.ListByKind(db.KindFact, 10)
	if len(facts) != 2 { // CreateTask's origin fact + oldFactID; the rejected new fact must not have been written
		t.Fatalf("got %d facts, want 2 (the rejected call must write nothing)", len(facts))
	}
}

func TestRecordFactSupersedesRequiresSettingOn(t *testing.T) {
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })
	FactSupersedesEnabled = func() bool { return false }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes setting off", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	oldFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "old blocking fact"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"summary": "new fact", "confidence": "observed", "evidence": "fresh session token works",
		"supersedes": []int64{oldFactID},
	})
	res, err := ts.recordFact().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "disabled") && !strings.Contains(res.Flatten(), "supersed") {
		t.Fatalf("expected a disabled-setting rejection, got: %s", res.Flatten())
	}
	facts, _ := store.ListByKind(db.KindFact, 10)
	if len(facts) != 2 { // CreateTask's origin fact + oldFactID
		t.Fatalf("got %d facts, want 2 (the rejected call must write nothing)", len(facts))
	}
}

func TestRecordFactSupersedesRejectsNonFactTarget(t *testing.T) {
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })
	FactSupersedesEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes bad target", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	intentID, err := store.AddIntent(map[string]any{"summary": "an intent, not a fact"}, 5, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"summary": "new fact", "confidence": "observed", "evidence": "proof",
		"supersedes": []int64{intentID},
	})
	res, err := ts.recordFact().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Flatten(), "does not exist") {
		t.Fatalf("expected a bad-target rejection naming the id, got (IsError=%v): %s", res.IsError, res.Flatten())
	}
	facts, _ := store.ListByKind(db.KindFact, 10)
	if len(facts) != 1 { // CreateTask's origin fact only
		t.Fatalf("got %d facts, want 1 (the rejected call must write nothing)", len(facts))
	}
}

func TestRecordFactSupersedesWritesEdgeOnSuccess(t *testing.T) {
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })
	FactSupersedesEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes success", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	oldFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "old: 403 looks like an authz gate"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"summary":    "new: the 403 was a wrong-credential-type artifact, a fresh session token works",
		"confidence": "observed", "evidence": "fresh session accessToken returned 200",
		"supersedes": []int64{oldFactID},
	})
	res, err := ts.recordFact().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Flatten(), "fact recorded: ") {
		t.Fatalf("expected success, got: %s", res.Flatten())
	}

	edges, err := store.Edges(0)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range edges {
		if e.Rel == db.RelSupersedes && e.To == oldFactID {
			found = true
		}
	}
	if !found {
		t.Fatalf("no supersedes edge -> old fact %d found in %+v", oldFactID, edges)
	}
}
