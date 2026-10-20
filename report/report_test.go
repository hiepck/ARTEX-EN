package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
)

func mkFinding(id int64, name, severity, summary string) *db.Node {
	payload, _ := json.Marshal(map[string]any{"name": name, "severity": severity, "summary": summary})
	return &db.Node{ID: id, Payload: payload}
}

func TestMarkdownOmitsNewSectionsWhenNoClustersOrReachability(t *testing.T) {
	md := Markdown(Input{
		Title: "t", GeneratedAt: time.Now(),
		Findings: []*db.Node{mkFinding(1, "XSS", "high", "stored xss")},
	})
	if strings.Contains(md, "Root-Cause Clusters") {
		t.Fatalf("report contains a Root-Cause Clusters section with no cluster data:\n%s", md)
	}
	if strings.Contains(md, "Asset Reachability") {
		t.Fatalf("report contains an Asset Reachability section with no reachability data:\n%s", md)
	}
	if strings.Contains(md, "Shares root cause with") {
		t.Fatalf("report contains a cross-reference line with no cluster data:\n%s", md)
	}
}

func TestMarkdownClusterOverviewAndPerFindingCiteSameSequenceNumbers(t *testing.T) {
	md := Markdown(Input{
		Title: "t", GeneratedAt: time.Now(),
		Findings: []*db.Node{
			mkFinding(101, "Heapdump leak", "high", "s1"),
			mkFinding(102, "Impersonation", "high", "s2"),
			mkFinding(103, "ATO", "high", "s3"),
		},
		Clusters: [][]int64{{101, 102, 103}},
	})
	if !strings.Contains(md, "## Root-Cause Clusters") {
		t.Fatalf("missing Root-Cause Clusters section:\n%s", md)
	}
	if !strings.Contains(md, "#1, #2, #3") {
		t.Fatalf("overview section does not cite #1, #2, #3:\n%s", md)
	}
	if !strings.Contains(md, "### 1.") || !strings.Contains(md, "**Shares root cause with:** #2, #3") {
		t.Fatalf("finding #1's cross-reference line missing or wrong:\n%s", md)
	}
	if !strings.Contains(md, "### 2.") || !strings.Contains(md, "**Shares root cause with:** #1, #3") {
		t.Fatalf("finding #2's cross-reference line missing or wrong:\n%s", md)
	}
	if !strings.Contains(md, "### 3.") || !strings.Contains(md, "**Shares root cause with:** #1, #2") {
		t.Fatalf("finding #3's cross-reference line missing or wrong:\n%s", md)
	}
}

func TestMarkdownClusterDropsUnresolvableMemberAndOmitsBelowThreshold(t *testing.T) {
	md := Markdown(Input{
		Title: "t", GeneratedAt: time.Now(),
		Findings: []*db.Node{
			mkFinding(201, "A", "high", "s1"),
			mkFinding(202, "B", "high", "s2"),
		},
		Clusters: [][]int64{
			{201, 202, 9999}, // 9999 does not exist in Findings -- dropped, 2 remain, cluster survives
			{201, 8888},      // 8888 does not exist -- only 1 resolvable member left, this cluster is omitted
		},
	})
	if !strings.Contains(md, "#1, #2") {
		t.Fatalf("surviving cluster {201,202} not rendered as #1, #2:\n%s", md)
	}
	if strings.Count(md, "- #") != 1 {
		t.Fatalf("expected exactly one cluster line (the below-threshold one must be omitted):\n%s", md)
	}
}

func TestMarkdownRendersReachabilityEdges(t *testing.T) {
	md := Markdown(Input{
		Title: "t", GeneratedAt: time.Now(),
		Findings:     []*db.Node{mkFinding(1, "X", "high", "s")},
		Reachability: []ReachabilityEdge{{SrcLabel: "host-a.example.com", DstLabel: "host-b.example.com"}},
	})
	if !strings.Contains(md, "## Asset Reachability") {
		t.Fatalf("missing Asset Reachability section:\n%s", md)
	}
	if !strings.Contains(md, "`host-a.example.com` can reach `host-b.example.com`") {
		t.Fatalf("reachability line missing or wrong:\n%s", md)
	}
}
