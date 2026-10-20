package db

import "sort"

// ClusterByRel groups node ids into connected components over edges whose Rel
// matches rel, via union-find. Only components with 2+ members are returned
// (a lone node with no partner isn't a cluster), each sorted ascending.
// Shared by agent/tools.go's graph_overview root_cause_clusters signal and
// report.Markdown's Root-Cause Clusters section -- the same grouping, two renderers.
func ClusterByRel(edges []Edge, rel string) [][]int64 {
	parent := map[int64]int64{}
	var find func(int64) int64
	find = func(x int64) int64 {
		if _, ok := parent[x]; !ok {
			parent[x] = x
		}
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int64) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}
	for _, e := range edges {
		if e.Rel != rel {
			continue
		}
		union(e.From, e.To)
	}
	groups := map[int64][]int64{}
	for id := range parent {
		groups[find(id)] = append(groups[find(id)], id)
	}
	var clusters [][]int64
	for _, members := range groups {
		if len(members) < 2 {
			continue
		}
		sort.Slice(members, func(i, j int) bool { return members[i] < members[j] })
		clusters = append(clusters, members)
	}
	return clusters
}
