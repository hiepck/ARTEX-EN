package agent

import (
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestGraphOverviewExposesAssetReachabilityOnlyWhenEnabled(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	assets := d.Assets()
	srcID, err := assets.UpsertRootDomain(db.UpsertRootDomainReq{Domain: "w2-task5-src.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer assets.DeleteByIDs([]int64{srcID})
	dstID, err := assets.UpsertRootDomain(db.UpsertRootDomainReq{Domain: "w2-task5-dst.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer assets.DeleteByIDs([]int64{dstID})

	task, err := d.CreateTaskWithOptions("reachability overview", "expose reachability to the planner", db.TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)

	store := d.Exploration(task.ExplorationID)
	if err := store.AddAssetEdge(srcID, db.AssetRelCanReach, dstID); err != nil {
		t.Fatal(err)
	}

	tools := NewToolSet(store, "planner")
	old := ReachabilityEdgesEnabled
	t.Cleanup(func() { ReachabilityEdgesEnabled = old })

	ReachabilityEdgesEnabled = func() bool { return false }
	off := tools.graphOverviewData()
	if _, has := off["asset_reachability"]; has {
		t.Fatalf("asset_reachability present while disabled: %+v", off["asset_reachability"])
	}

	ReachabilityEdgesEnabled = func() bool { return true }
	on := tools.graphOverviewData()
	reach, ok := on["asset_reachability"].([]map[string]any)
	if !ok || len(reach) != 1 {
		t.Fatalf("asset_reachability = %#v, want exactly 1 entry", on["asset_reachability"])
	}
	if reach[0]["src_asset_id"] != srcID || reach[0]["dst_asset_id"] != dstID || reach[0]["rel"] != db.AssetRelCanReach {
		t.Fatalf("entry = %+v, want {%d %s %d}", reach[0], srcID, db.AssetRelCanReach, dstID)
	}
}
