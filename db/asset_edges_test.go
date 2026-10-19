package db

import "testing"

func TestAssetEdgesRoundTripIsolatesAndDedups(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) - skipping", err)
	}
	defer d.Close()

	assets := d.Assets()
	srcID, err := assets.UpsertRootDomain(UpsertRootDomainReq{Domain: "w2-reach-src.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer assets.DeleteByIDs([]int64{srcID})
	dstID, err := assets.UpsertRootDomain(UpsertRootDomainReq{Domain: "w2-reach-dst.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer assets.DeleteByIDs([]int64{dstID})

	expID, err := d.CreateExploration("asset edges", "reachability round trip")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM explorations WHERE id=$1`, expID)
	store := d.Exploration(expID)

	otherExpID, err := d.CreateExploration("asset edges other", "foreign exploration must not see this edge")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM explorations WHERE id=$1`, otherExpID)
	otherStore := d.Exploration(otherExpID)

	// Write the same edge twice -- must dedup, not duplicate or error.
	if err := store.AddAssetEdge(srcID, AssetRelCanReach, dstID); err != nil {
		t.Fatal(err)
	}
	if err := store.AddAssetEdge(srcID, AssetRelCanReach, dstID); err != nil {
		t.Fatal(err)
	}

	got, err := store.AssetEdges(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d edges, want exactly 1 (deduped): %+v", len(got), got)
	}
	if got[0].SrcAssetID != srcID || got[0].Rel != AssetRelCanReach || got[0].DstAssetID != dstID {
		t.Fatalf("edge = %+v, want {%d %s %d}", got[0], srcID, AssetRelCanReach, dstID)
	}

	otherGot, err := otherStore.AssetEdges(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherGot) != 0 {
		t.Fatalf("foreign exploration sees the edge (isolation violated): %+v", otherGot)
	}

	if err := store.AddAssetEdge(srcID, AssetRelCanReach, srcID); err == nil {
		t.Fatal("self-edge was accepted, want a CHECK-constraint rejection")
	}
}
