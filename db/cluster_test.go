package db

import "testing"

func TestClusterByRelCollapsesTransitiveChain(t *testing.T) {
	edges := []Edge{
		{From: 2, Rel: "shares_root_cause", To: 1},
		{From: 3, Rel: "shares_root_cause", To: 2},
	}
	clusters := ClusterByRel(edges, "shares_root_cause")
	if len(clusters) != 1 || len(clusters[0]) != 3 {
		t.Fatalf("ClusterByRel = %v, want exactly 1 cluster of 3 members", clusters)
	}
	want := []int64{1, 2, 3}
	for i, id := range clusters[0] {
		if id != want[i] {
			t.Fatalf("cluster members = %v, want %v (sorted ascending)", clusters[0], want)
		}
	}
}

func TestClusterByRelIgnoresLoneNodeAndOtherRel(t *testing.T) {
	edges := []Edge{
		{From: 2, Rel: "shares_root_cause", To: 1},
		{From: 5, Rel: "supersedes", To: 4}, // different rel -- must not merge into any cluster
	}
	clusters := ClusterByRel(edges, "shares_root_cause")
	if len(clusters) != 1 || len(clusters[0]) != 2 {
		t.Fatalf("ClusterByRel = %v, want exactly 1 cluster of 2 members (the supersedes edge must be ignored)", clusters)
	}
}

func TestClusterByRelNoMatchingEdgesReturnsEmpty(t *testing.T) {
	edges := []Edge{{From: 1, Rel: "yields", To: 2}}
	clusters := ClusterByRel(edges, "shares_root_cause")
	if len(clusters) != 0 {
		t.Fatalf("ClusterByRel = %v, want no clusters", clusters)
	}
}
