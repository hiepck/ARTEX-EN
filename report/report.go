// Package report renders a deliverable Markdown report from the exploration
// graph: confirmed findings (with severity/evidence) (docs §15 P5).
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Autumn-27/artex/db"
)

// Input bundles what the report needs.
type Input struct {
	Title        string
	Goal         string
	GeneratedAt  time.Time
	AssetCounts  map[string]int
	Findings     []*db.Node
	Clusters     [][]int64          // each entry: exploration-node ids of findings sharing one root cause (2+ members)
	Reachability []ReachabilityEdge // pre-resolved, human-readable; empty/nil when none
}

// ReachabilityEdge is one already-labeled can_reach relationship, ready to render.
type ReachabilityEdge struct {
	SrcLabel string
	DstLabel string
}

type findingView struct {
	ID        int64
	VulnClass string
	Name      string
	Severity  string
	Summary   string
	PoC       string
}

func parseFinding(n *db.Node) findingView {
	var p struct {
		VulnClass string `json:"vulnclass"`
		Name      string `json:"name"`
		Severity  string `json:"severity"`
		Summary   string `json:"summary"`
		Evidence  struct {
			PoC string `json:"poc"`
		} `json:"evidence"`
	}
	_ = json.Unmarshal(n.Payload, &p)
	return findingView{ID: n.ID, VulnClass: p.VulnClass, Name: p.Name, Severity: p.Severity, Summary: p.Summary, PoC: p.Evidence.PoC}
}

var sevRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "": 4}

// Markdown renders the report.
func Markdown(in Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Penetration test report - %s\n\n", nz(in.Title, "Untitled task"))
	fmt.Fprintf(&b, "- **Task goal**: %s\n", nz(in.Goal, "(unspecified)"))
	fmt.Fprintf(&b, "- **Generated at**: %s\n\n", in.GeneratedAt.Format("2006-01-02 15:04:05"))

	// summary
	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "- Confirmed findings: **%d**\n", len(in.Findings))
	fmt.Fprintf(&b, "- Assets: ")
	var types []string
	for t := range in.AssetCounts {
		types = append(types, t)
	}
	sort.Strings(types)
	for i, t := range types {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s %d", t, in.AssetCounts[t])
	}
	b.WriteString("\n\n")

	var fs []findingView
	if len(in.Findings) > 0 {
		fs = make([]findingView, 0, len(in.Findings))
		for _, n := range in.Findings {
			fs = append(fs, parseFinding(n))
		}
		sort.SliceStable(fs, func(i, j int) bool { return sevRank[fs[i].Severity] < sevRank[fs[j].Severity] })
	}

	seqOf := make(map[int64]int, len(fs))
	for i, f := range fs {
		seqOf[f.ID] = i + 1
	}

	var clusterLines []string
	siblingsOf := map[int64][]int{}
	for _, members := range in.Clusters {
		var nums []int
		for _, id := range members {
			if n, ok := seqOf[id]; ok {
				nums = append(nums, n)
			}
		}
		if len(nums) < 2 {
			continue
		}
		sort.Ints(nums)
		var parts []string
		for _, n := range nums {
			parts = append(parts, fmt.Sprintf("#%d", n))
		}
		clusterLines = append(clusterLines, strings.Join(parts, ", "))
		for _, id := range members {
			if _, ok := seqOf[id]; !ok {
				continue
			}
			for _, other := range members {
				if other == id {
					continue
				}
				if n, ok := seqOf[other]; ok {
					siblingsOf[id] = append(siblingsOf[id], n)
				}
			}
		}
	}
	if len(clusterLines) > 0 {
		fmt.Fprintf(&b, "## Root-Cause Clusters\n\n")
		fmt.Fprintf(&b, "These findings were explicitly linked by a worker as tracing back to the same underlying flaw:\n\n")
		for _, line := range clusterLines {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	if len(in.Reachability) > 0 {
		fmt.Fprintf(&b, "## Asset Reachability\n\n")
		fmt.Fprintf(&b, "Confirmed blast-radius relationships between assets (one asset's access reaches another):\n\n")
		for _, e := range in.Reachability {
			fmt.Fprintf(&b, "- `%s` can reach `%s`\n", e.SrcLabel, e.DstLabel)
		}
		b.WriteString("\n")
	}

	// findings
	fmt.Fprintf(&b, "## Findings\n\n")
	if len(fs) == 0 {
		b.WriteString("_No findings were confirmed in this run._\n\n")
	} else {
		for i, f := range fs {
			fmt.Fprintf(&b, "### %d. [%s] %s\n\n", i+1, strings.ToUpper(nz(f.Severity, "info")), nz(f.Name, nz(f.VulnClass, "Uncategorized")))
			if sibs := siblingsOf[f.ID]; len(sibs) > 0 {
				sort.Ints(sibs)
				var parts []string
				for _, n := range sibs {
					parts = append(parts, fmt.Sprintf("#%d", n))
				}
				fmt.Fprintf(&b, "**Shares root cause with:** %s\n\n", strings.Join(parts, ", "))
			}
			fmt.Fprintf(&b, "%s\n\n", nz(f.Summary, ""))
			if f.PoC != "" {
				fmt.Fprintf(&b, "**PoC / evidence:**\n\n```\n%s\n```\n\n", f.PoC)
			}
		}
	}

	return b.String()
}

func nz(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
