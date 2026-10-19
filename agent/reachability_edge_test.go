package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// stubFindingRecorder always succeeds with a canned finding, regardless of
// input, so these tests exercise addFinding's own reaches-specific logic in
// isolation from the real evidence.Store's traffic-lookup mechanics (already
// covered by other tests).
type stubFindingRecorder struct{ calls int }

func (s *stubFindingRecorder) Record(context.Context, db.RecordFindingInput, []db.TrafficRef) (*db.RecordedFinding, error) {
	s.calls++
	return &db.RecordedFinding{FindingID: 999, NodeID: 1000, Traffic: &db.FindingTraffic{}}, nil
}

func TestReportFindingReachesRequiresEvidence(t *testing.T) {
	old := ReachabilityEdgesEnabled
	t.Cleanup(func() { ReachabilityEdgesEnabled = old })
	ReachabilityEdgesEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("reaches no evidence", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)

	ts := NewToolSet(d.Exploration(task.ExplorationID), "")
	ts.taskID = task.ID
	rec := &stubFindingRecorder{}
	ts.SetFindingRecorder(rec)

	res, err := ts.addFinding().Call(t.Context(), json.RawMessage(`{"vulnclass":"SSRF","severity":"high","summary":"ssrf fixture","asset_ids":[1],"reaches":[2]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "evidence") {
		t.Fatalf("expected an evidence-required rejection, got: %s", res.Flatten())
	}
	if rec.calls != 0 {
		t.Fatalf("recorder was called despite missing evidence (calls=%d) — the whole report must be rejected before any write", rec.calls)
	}
}

func TestReportFindingReachesRequiresSettingOn(t *testing.T) {
	old := ReachabilityEdgesEnabled
	t.Cleanup(func() { ReachabilityEdgesEnabled = old })
	ReachabilityEdgesEnabled = func() bool { return false }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("reaches setting off", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)

	ts := NewToolSet(d.Exploration(task.ExplorationID), "")
	ts.taskID = task.ID
	rec := &stubFindingRecorder{}
	ts.SetFindingRecorder(rec)

	res, err := ts.addFinding().Call(t.Context(), json.RawMessage(`{"vulnclass":"SSRF","severity":"high","summary":"ssrf fixture","asset_ids":[1],"reaches":[2],"traffic_refs":[{"traffic_id":"whatever"}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "disabled") && !strings.Contains(res.Flatten(), "reachability") {
		t.Fatalf("expected a disabled-setting rejection, got: %s", res.Flatten())
	}
	if rec.calls != 0 {
		t.Fatalf("recorder was called despite the setting being off (calls=%d)", rec.calls)
	}
}

func TestReportFindingReachesWritesEdgeOnSuccess(t *testing.T) {
	old := ReachabilityEdgesEnabled
	t.Cleanup(func() { ReachabilityEdgesEnabled = old })
	ReachabilityEdgesEnabled = func() bool { return true }
	oldTB := FindingTrafficBindingEnabled
	t.Cleanup(func() { FindingTrafficBindingEnabled = oldTB })
	FindingTrafficBindingEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	assets := d.Assets()
	srcID, err := assets.UpsertRootDomain(db.UpsertRootDomainReq{Domain: "w2-task3-src.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer assets.DeleteByIDs([]int64{srcID})
	dstID, err := assets.UpsertRootDomain(db.UpsertRootDomainReq{Domain: "w2-task3-dst.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer assets.DeleteByIDs([]int64{dstID})

	task, err := d.CreateTask("reaches success", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)

	store := d.Exploration(task.ExplorationID)
	ts := NewToolSet(store, "")
	ts.taskID = task.ID
	rec := &stubFindingRecorder{}
	ts.SetFindingRecorder(rec)

	body, _ := json.Marshal(map[string]any{
		"vulnclass": "SSRF", "severity": "high", "summary": "ssrf fixture",
		"asset_ids":    []int64{srcID, dstID}, // dstID included deliberately: the (dstID,dstID) self-pair must be skipped
		"reaches":      []int64{dstID},
		"traffic_refs": []map[string]any{{"traffic_id": "whatever-syntactically-valid"}},
	})
	res, err := ts.addFinding().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Flatten(), "finding recorded: ") {
		t.Fatalf("expected success, got: %s", res.Flatten())
	}
	if rec.calls != 1 {
		t.Fatalf("recorder calls = %d, want 1", rec.calls)
	}

	edges, err := store.AssetEdges(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 1 {
		t.Fatalf("got %d edges, want exactly 1 (srcID->dstID; the dstID->dstID self-pair must be skipped): %+v", len(edges), edges)
	}
	if edges[0].SrcAssetID != srcID || edges[0].DstAssetID != dstID || edges[0].Rel != db.AssetRelCanReach {
		t.Fatalf("edge = %+v, want {%d %s %d}", edges[0], srcID, db.AssetRelCanReach, dstID)
	}
}
