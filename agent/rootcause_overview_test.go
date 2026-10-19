package agent

import (
	"sort"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestGraphOverviewExposesRootCauseClustersTransitively(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("root cause overview", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)

	f1, err := store.AddNode(db.KindFinding, map[string]any{"summary": "heapdump leak"}, 9, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := store.AddNode(db.KindFinding, map[string]any{"summary": "impersonation via the leaked secret"}, 9, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	f3, err := store.AddNode(db.KindFinding, map[string]any{"summary": "open-banking ATO via the same secret"}, 9, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddNode(db.KindFinding, map[string]any{"summary": "unrelated finding"}, 9, "confirmed", "worker", nil); err != nil {
		t.Fatal(err) // a lone, unlinked finding -- must never surface as a 1-member cluster
	}

	tools := NewToolSet(store, "planner")
	old := RootCauseGroupingEnabled
	t.Cleanup(func() { RootCauseGroupingEnabled = old })

	RootCauseGroupingEnabled = func() bool { return false }
	off := tools.graphOverviewData()
	if _, has := off["root_cause_clusters"]; has {
		t.Fatalf("root_cause_clusters present while disabled: %+v", off["root_cause_clusters"])
	}

	RootCauseGroupingEnabled = func() bool { return true }
	onEmpty := tools.graphOverviewData()
	if clusters, ok := onEmpty["root_cause_clusters"].([]map[string]any); !ok || len(clusters) != 0 {
		t.Fatalf("root_cause_clusters with no edges yet = %#v, want an empty slice, not absent/nil", onEmpty["root_cause_clusters"])
	}

	if err := store.Link(f2, db.RelSharesRootCause, f1); err != nil {
		t.Fatal(err)
	}
	if err := store.Link(f3, db.RelSharesRootCause, f2); err != nil {
		t.Fatal(err)
	}

	on := tools.graphOverviewData()
	clusters, ok := on["root_cause_clusters"].([]map[string]any)
	if !ok || len(clusters) != 1 {
		t.Fatalf("root_cause_clusters = %#v, want exactly 1 cluster (f1,f2,f3 transitively linked; the lone finding must not appear)", on["root_cause_clusters"])
	}
	ids, ok := clusters[0]["finding_ids"].([]int64)
	if !ok || len(ids) != 3 {
		t.Fatalf("cluster finding_ids = %#v, want 3 members", clusters[0]["finding_ids"])
	}
	want := []int64{f1, f2, f3}
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	for i, id := range ids {
		if id != want[i] {
			t.Fatalf("cluster finding_ids = %v, want %v (sorted ascending)", ids, want)
		}
	}
}
