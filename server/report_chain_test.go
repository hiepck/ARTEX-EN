package server

import (
	"context"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestGetReportIncludesClustersAndReachability(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	td := t.TempDir()
	s := New(context.Background(), m, td, td, td)

	task, err := m.CreateTask("report chain fixture", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	tid, _ := strconv.ParseInt(task.ID, 10, 64)
	defer m.pg.DeleteTask(tid)
	store := m.pg.Exploration(task.ExpID)

	f1, err := store.AddNode(db.KindFinding, map[string]any{"name": "Heapdump leak", "severity": "high", "summary": "s1"}, 9, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	f2, err := store.AddNode(db.KindFinding, map[string]any{"name": "Impersonation", "severity": "high", "summary": "s2"}, 9, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Link(f2, db.RelSharesRootCause, f1); err != nil {
		t.Fatal(err)
	}

	srcID, err := m.Assets().UpsertRootDomain(db.UpsertRootDomainReq{Domain: "report-chain-src.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Assets().DeleteByIDs([]int64{srcID})
	dstID, err := m.Assets().UpsertRootDomain(db.UpsertRootDomainReq{Domain: "report-chain-dst.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Assets().DeleteByIDs([]int64{dstID})
	if err := store.AddAssetEdge(srcID, db.AssetRelCanReach, dstID); err != nil {
		t.Fatal(err)
	}

	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/report?task="+task.ID, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET /api/report failed %d: %s", w.Code, w.Body)
	}
	md := w.Body.String()
	if !strings.Contains(md, "## Root-Cause Clusters") {
		t.Fatalf("report is missing the Root-Cause Clusters section:\n%s", md)
	}
	if !strings.Contains(md, "Shares root cause with") {
		t.Fatalf("report is missing the per-finding cross-reference:\n%s", md)
	}
	if !strings.Contains(md, "## Asset Reachability") {
		t.Fatalf("report is missing the Asset Reachability section:\n%s", md)
	}
	if !strings.Contains(md, "`report-chain-src.invalid` can reach `report-chain-dst.invalid`") {
		t.Fatalf("report does not render the resolved reachability edge with real asset labels:\n%s", md)
	}
}
