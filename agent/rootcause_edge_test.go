package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestReportFindingSharesRootCauseRequiresSettingOn(t *testing.T) {
	old := RootCauseGroupingEnabled
	t.Cleanup(func() { RootCauseGroupingEnabled = old })
	RootCauseGroupingEnabled = func() bool { return false }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("root cause setting off", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	oldFindingID, err := store.AddNode(db.KindFinding, map[string]any{"summary": "heapdump leak"}, 9, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"vulnclass": "SSRF", "severity": "high", "summary": "new finding",
		"shares_root_cause_with": []int64{oldFindingID},
	})
	res, err := ts.addFinding().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "disabled") && !strings.Contains(res.Flatten(), "root-cause") {
		t.Fatalf("expected a disabled-setting rejection, got: %s", res.Flatten())
	}
	findings, _ := store.ListByKind(db.KindFinding, 10)
	if len(findings) != 1 { // only oldFindingID; the rejected new finding must not have been written
		t.Fatalf("got %d findings, want 1 (the rejected call must write nothing)", len(findings))
	}
}

func TestReportFindingSharesRootCauseRejectsNonFindingTarget(t *testing.T) {
	old := RootCauseGroupingEnabled
	t.Cleanup(func() { RootCauseGroupingEnabled = old })
	RootCauseGroupingEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("root cause bad target", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	intentID, err := store.AddIntent(map[string]any{"summary": "an intent, not a finding"}, 5, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"vulnclass": "SSRF", "severity": "high", "summary": "new finding",
		"shares_root_cause_with": []int64{intentID},
	})
	res, err := ts.addFinding().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Flatten(), "does not exist") {
		t.Fatalf("expected a bad-target rejection naming the id, got (IsError=%v): %s", res.IsError, res.Flatten())
	}
	findings, _ := store.ListByKind(db.KindFinding, 10)
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want 0 (the rejected call must write nothing)", len(findings))
	}
}

func TestReportFindingSharesRootCauseWritesEdgeOnSuccess(t *testing.T) {
	old := RootCauseGroupingEnabled
	t.Cleanup(func() { RootCauseGroupingEnabled = old })
	RootCauseGroupingEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("root cause success", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	oldFindingID, err := store.AddNode(db.KindFinding, map[string]any{"summary": "heapdump leak"}, 9, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"vulnclass": "Broken Access Control", "severity": "critical",
		"summary":                "admin impersonation via the leaked keycloak secret",
		"shares_root_cause_with": []int64{oldFindingID},
	})
	res, err := ts.addFinding().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Flatten(), "finding recorded: ") {
		t.Fatalf("expected success, got: %s", res.Flatten())
	}

	edges, err := store.Edges(0)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range edges {
		if e.Rel == db.RelSharesRootCause && e.To == oldFindingID {
			found = true
		}
	}
	if !found {
		t.Fatalf("no shares_root_cause edge -> old finding %d found in %+v", oldFindingID, edges)
	}
}
