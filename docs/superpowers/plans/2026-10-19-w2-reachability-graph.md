# W2: Evidence-Gated Asset Reachability Graph Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `report_finding` optionally declare that the finding's asset(s) can reach other assets (`reaches`), persisted as a first-class `can_reach` edge in a new `asset_edges` table, visible to the planner in `graph_overview` — but only when the claim carries real, verified evidence (the same discipline `bind_finding_traffic` already hard-requires), never on the honor system. Scoped to `report_finding` only for this round (`insert_assets` is explicitly deferred — see Non-goals) and gated behind a new `reachability_edges_enabled` setting, default off.

**Architecture:** A new, purpose-built `asset_edges` table (additive, composite-PK, FK'd to `assets(id)` — not an extension of `exploration_edges`, which is scoped to reasoning nodes, not assets) records `(exploration_id, src_asset_id, rel, dst_asset_id)`. `report_finding`'s handler gains an optional `reaches` parameter; when present, it requires the SAME evidence resolution the finding's own `traffic_refs`/`evidence_hint_id` already produce (`t.findingRefsFromHint`) to be non-empty — a `reaches` claim with no evidence is rejected outright, not silently dropped, mirroring `bind_finding_traffic`'s existing hard-reject contract instead of `report_finding`'s own (optional) evidence discipline, since the latter turned out not to be a hard gate at all. On success, edges are written via a new `ExplorationStore` method. `graphOverviewData()` surfaces them to the planner as a new top-level key, gated by the same setting. The planner prompt is updated to treat a new reachability edge as a route-diversity signal it already knows how to act on.

**Tech Stack:** Go, PostgreSQL (`database/sql` via `pgx/v5/stdlib`), the existing `agent`/`db`/`server` packages.

**Spec:** `docs/superpowers/specs/2026-10-17-core-engine-finding-capability-design.md` (workstream W2), as refined by this plan's own feasibility research (see Global Constraints for the corrections).

## Global Constraints

- **Scope correction from the original spec**: the spec's W2 section assumed `report_finding` AND `insert_assets` would both write edges, reusing `report_finding`'s "existing evidence-binding discipline" as the hard gate. Feasibility research found `report_finding`'s own `traffic_refs`/`evidence_hint_id` are **optional** today (not a hard gate at all — the only existing hard-reject-if-empty evidence check in the codebase is the separate, server-side `bind_finding_traffic` tool), and `insert_assets` has **zero** evidence-binding infrastructure. The owner decided (this round): **`report_finding` only**, modeling the new hard-reject check on `bind_finding_traffic`'s contract, not on `report_finding`'s own historically-optional one. `insert_assets`-based edge-writing is explicitly deferred.
- **No new cross-package bridge**: this plan must not add a new `agent`↔`server` evidence-validation bridge (e.g. a new interface or function-variable reaching into `evidence.Store`). It reuses the `FindingRecorder` interface and `t.findingRefsFromHint`/`db.NormalizeTrafficRefs` plumbing `report_finding` already has. If a task seems to need new cross-package evidence plumbing, that's a signal this plan or the spec is wrong, not a passing deviation.
- **`asset_edges` is its own table**, not an extension of `exploration_edges`'s `rel` CHECK — `exploration_edges` is scoped to reasoning nodes (`exploration_nodes`), asset edges need a different FK target (`assets(id)`) entirely incompatible with that table's `src_id`/`dst_id` columns.
- **House style for the new table**: `CREATE TABLE IF NOT EXISTS`, composite primary key (mirrors `exploration_edges`'s `(exploration_id, src_id, rel, dst_id)` PK exactly, substituting asset ids), `REFERENCES assets(id) ON DELETE CASCADE` for both endpoints (the confirmed house convention for any hard reference to an asset row — see `exploration_anchors.asset_id`, `task_asset_links.asset_id`), `REFERENCES explorations(id) ON DELETE CASCADE` for scoping. A `CHECK` constraint for both the relation vocabulary and a no-self-edge guard, mirroring `exploration_edges`'s `ck_edge_rel`/`ck_edge_noself`.
- **Opt-in, default off, independently toggleable**: `reachability_edges_enabled` is its own setting (not reused from `agent_traffic_binding`), following the exact `noa_compaction`/`burn_signal_enabled` feature-flag template — **including the settings HTTP API (GET payload + PATCH handler) from the start**, not deferred to a later fix (W1's final review caught exactly this gap after the fact; this plan builds it into Task 2 directly).
- **Fail closed, not silently dropped**: when `reaches` is present but no evidence resolves (or the setting is off), the whole `report_finding` call must be REJECTED before any write happens — this is the opposite of how `report_finding`'s own `traffic_refs`/`evidence_hint_id` behave today (silently zeroed when the setting is off). Do not reuse that silent-drop behavior for `reaches`.
- **Relation vocabulary namespace**: the new constant (`db.AssetRelCanReach = "can_reach"`) goes in its own, clearly-commented `const` block in `db/constants.go`, separate from the existing `exploration_edges`-scoped `Rel*` block, to avoid implying it belongs to the same CHECK vocabulary.
- **Planner visibility is not automatic**: exposing the new table requires an explicit new fetch + a new key in `graphOverviewData()`'s output — there is no existing "assets have edges" plumbing anywhere in the planner-visible surface to extend (confirmed: `node_detail` renders no edges at all; `list_assets` carries no cross-asset relation data).

## Review Focus

- **A `reaches` claim with no evidence must be REJECTED, not silently accepted or silently stripped** — the single most important behavior this plan exists to get right. A test that only checks "reaches is written when evidence is present" without also checking "the whole call fails when it is absent" would miss the actual point.
- **Self-reference and duplicate edges**: `reaches` containing the same asset id as `asset_ids` (a self-loop) must not reach the database as a CHECK-constraint violation that aborts an otherwise-successful finding write — it must be filtered out before the write, quietly, since the finding itself is more important than a degenerate edge. Re-reporting the same edge twice must be a no-op, not a duplicate row or an error.
- **The flag is off (the default)**: `graphOverviewData()` must not run the new query and must not add any reachability key to its output; `report_finding`'s schema must not advertise `reaches` at all (stripped, matching the existing `stripTrafficParameters` pattern for `traffic_refs`/`evidence_hint_id`).
- **Cross-exploration isolation**: an asset edge recorded in one exploration must never be visible to a different exploration's `graph_overview`, even if both explorations happen to reference the same underlying asset row (assets are shared/global, per `assets.task_ids`, while `asset_edges` is exploration-scoped).
- **The settings HTTP API must exist from the first commit of Task 2** — not deferred to a final-review fix wave, unlike what happened in the W1 workstream for its own flag.

---

### Task 1: `asset_edges` table + `db.AssetRelCanReach` + `ExplorationStore.AddAssetEdge`/`AssetEdges`

**Files:**
- Modify: `db/schema.sql` (new table, placed near `exploration_edges`/`exploration_anchors`, `db/schema.sql:249-282`)
- Modify: `db/constants.go` (new, separate `const` block near the existing `Rel*` block, `db/constants.go:33-40`)
- Modify: `db/exploration.go` (new `AssetEdge` type + `AddAssetEdge`/`AssetEdges` methods, placed near `Edges`, `db/exploration.go:985-1003`)
- Test: `db/asset_edges_test.go` (new)

**Interfaces:**
- Consumes: `(s *AssetStore) UpsertRootDomain(req UpsertRootDomainReq) (int64, error)` (`db/assets.go:255`) and `UpsertIP(req UpsertIPReq) (int64, error)` (`db/assets.go:338`) for test fixtures; `(d *DB) CreateExploration(description, goal string) (int64, error)` (`db/exploration.go:178`); `testDSN(t *testing.T) string` (`db/db_test.go:56`).
- Produces: `type AssetEdge struct { SrcAssetID int64; Rel string; DstAssetID int64 }`; `const AssetRelCanReach = "can_reach"`; `func (s *ExplorationStore) AddAssetEdge(srcAssetID int64, rel string, dstAssetID int64) error` (idempotent — a duplicate `(exploration_id, src, rel, dst)` is a safe no-op via `ON CONFLICT DO NOTHING`, relying on the composite primary key); `func (s *ExplorationStore) AssetEdges(limit int) ([]AssetEdge, error)` (scoped to `s.expID`, mirrors `Edges(limit)`'s shape exactly, default limit 5000 when `limit<=0`). Used by Tasks 3 and 5.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestAssetEdgesRoundTripIsolatesAndDedups -v`
Expected: FAIL (`AssetRelCanReach`/`AddAssetEdge`/`AssetEdges` undefined, or a `relation "asset_edges" does not exist` error once the Go side compiles against a schema that doesn't have the table yet — whichever comes first)

- [ ] **Step 3: Add the schema, the constant, and the two methods**

In `db/schema.sql`, near `exploration_edges`/`exploration_anchors`:
```sql
CREATE TABLE IF NOT EXISTS asset_edges (
    exploration_id BIGINT NOT NULL REFERENCES explorations(id) ON DELETE CASCADE,
    src_asset_id   BIGINT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    rel            TEXT NOT NULL,
    dst_asset_id   BIGINT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (exploration_id, src_asset_id, rel, dst_asset_id),
    CONSTRAINT ck_asset_edge_noself CHECK (src_asset_id <> dst_asset_id),
    CONSTRAINT ck_asset_edge_rel CHECK (rel IN ('can_reach'))
);
CREATE INDEX IF NOT EXISTS idx_assetedges_src ON asset_edges(src_asset_id, rel);
CREATE INDEX IF NOT EXISTS idx_assetedges_dst ON asset_edges(dst_asset_id, rel);
```

In `db/constants.go`, a new block separate from the `exploration_edges`-scoped `Rel*` block:
```go
// Asset-edge relations (asset_edges.rel) -- a different table and vocabulary
// from the exploration_edges Rel* constants above; do not conflate the two.
const (
	AssetRelCanReach = "can_reach"
)
```

In `db/exploration.go`, near `Edges`:
```go
// AssetEdge is one row from asset_edges (reachability relations between two
// assets, scoped per exploration).
type AssetEdge struct {
	SrcAssetID int64
	Rel        string
	DstAssetID int64
}

func (s *ExplorationStore) AddAssetEdge(srcAssetID int64, rel string, dstAssetID int64) error {
	_, err := s.db.Exec(`INSERT INTO asset_edges(exploration_id, src_asset_id, rel, dst_asset_id)
		VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, s.expID, srcAssetID, rel, dstAssetID)
	return err
}

func (s *ExplorationStore) AssetEdges(limit int) ([]AssetEdge, error) {
	if limit <= 0 {
		limit = 5000
	}
	rows, err := s.db.Query(`SELECT src_asset_id, rel, dst_asset_id FROM asset_edges WHERE exploration_id=$1 LIMIT $2`, s.expID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssetEdge
	for rows.Next() {
		var e AssetEdge
		if err := rows.Scan(&e.SrcAssetID, &e.Rel, &e.DstAssetID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestAssetEdgesRoundTripIsolatesAndDedups -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add db/schema.sql db/constants.go db/exploration.go db/asset_edges_test.go
git commit -m "feat(db): add asset_edges table + AddAssetEdge/AssetEdges"
```

---

### Task 2: Feature flag `reachability_edges_enabled` (Manager + settings API + agent wiring)

**Files:**
- Modify: `server/manager.go` (setting constant + `Manager.ReachabilityEdgesEnabled()/SetReachabilityEdgesEnabled()`, near `settingBurnSignalEnabled`)
- Modify: `server/server.go` (GET settings payload + PATCH settings request/handler, near the `burn_signal_enabled` lines added in the W1 workstream)
- Modify: `server/assembly.go` (wire `agent.ReachabilityEdgesEnabled` in `wireTools`, near `agent.BurnSignalEnabled`)
- Create: `agent/reachability.go` (the package-level function variable + nil-safe accessor, mirroring `agent/burn_signal.go` exactly)
- Test: `server/reachability_test.go` (new, mirroring `server/burn_signal_test.go`'s two tests — Manager getter/setter, and a settings-API round trip via `trafficEvidenceServer`/`s.Handler()`), `agent/reachability_test.go` (new, mirroring `agent/burn_signal_test.go`)

**Interfaces:**
- Consumes: `(*db.DB).GetBool(key string, def bool) bool` / `SetBool(key string, val bool) error` (`db/settings.go:50,59`); `NewManager(dir, proxyAddr string) (*Manager, error)` (`server/manager.go:344`); the `trafficEvidenceServer(t) (*Server, *db.RecordedFinding, func(string,string,string) *httptest.ResponseRecorder)` fixture (`server/finding_traffic_test.go:23`) for the HTTP round-trip test, following the exact pattern `server/burn_signal_test.go`'s `TestBurnSignalEnabledSettingsAPIRoundTrip` already established in the W1 workstream.
- Produces: `Manager.ReachabilityEdgesEnabled() bool` / `SetReachabilityEdgesEnabled(on bool) error`; `agent.ReachabilityEdgesEnabled func() bool` + unexported `reachabilityEdgesEnabled() bool`; the setting reachable via `GET /api/settings` (`"reachability_edges_enabled"` key) and `PUT /api/settings` (`{"reachability_edges_enabled": bool}`). Used by Tasks 3, 4, 5.

- [ ] **Step 1: Write the failing Manager-level test**

```go
func TestReachabilityEdgesEnabledDefaultsOffAndPersists(t *testing.T) {
    m, err := NewManager(t.TempDir(), "")
    if err != nil {
        t.Skipf("postgres unavailable (%v) — skipping", err)
    }
    defer m.Close()
    clearSetting := func() { _, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, "reachability_edges_enabled") }
    clearSetting()
    t.Cleanup(clearSetting)

    if m.ReachabilityEdgesEnabled() {
        t.Fatal("ReachabilityEdgesEnabled() default = true, want false")
    }
    if err := m.SetReachabilityEdgesEnabled(true); err != nil {
        t.Fatalf("SetReachabilityEdgesEnabled: %v", err)
    }
    if !m.ReachabilityEdgesEnabled() {
        t.Fatal("ReachabilityEdgesEnabled() after Set(true) = false, want true")
    }
}
```

(The explicit `DELETE FROM settings` up front is required per the W1 workstream's own ledgered finding: this setting lives in a shared table that persists across test runs against the same Postgres instance.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestReachabilityEdgesEnabledDefaultsOffAndPersists -v`
Expected: FAIL (`m.ReachabilityEdgesEnabled` undefined)

- [ ] **Step 3: Implement the Manager-level setting**

In `server/manager.go`, add `settingReachabilityEdges = "reachability_edges_enabled"` next to `settingBurnSignalEnabled`, and:
```go
func (m *Manager) ReachabilityEdgesEnabled() bool {
	return m.pg.GetBool(settingReachabilityEdges, false)
}

func (m *Manager) SetReachabilityEdgesEnabled(on bool) error {
	return m.pg.SetBool(settingReachabilityEdges, on)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/ -run TestReachabilityEdgesEnabledDefaultsOffAndPersists -v`
Expected: PASS

- [ ] **Step 5: Write the failing settings-API round-trip test**

Follow `server/burn_signal_test.go`'s `TestBurnSignalEnabledSettingsAPIRoundTrip` exactly (same `trafficEvidenceServer(t)` fixture, same `readBurnSignal`-style closure pattern renamed for this setting): a GET to `/api/settings` must include `"reachability_edges_enabled": false` by default; a `PUT /api/settings` with `{"reachability_edges_enabled":true}` must succeed; a subsequent GET must reflect `true`; `s.m.ReachabilityEdgesEnabled()` must agree.

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./server/ -run TestReachabilityEdgesEnabledSettingsAPIRoundTrip -v`
Expected: FAIL (missing field in the GET payload / PATCH struct)

- [ ] **Step 7: Wire the settings HTTP API**

In `server/server.go`'s GET settings payload map (near the `"burn_signal_enabled"` line added in the W1 workstream): add `"reachability_edges_enabled": s.m.ReachabilityEdgesEnabled(),`. In the PATCH/PUT settings request struct and handler (near the `BurnSignalEnabled *bool` field and its `if req.BurnSignalEnabled != nil {...}` block): add a sibling `ReachabilityEdgesEnabled *bool \`json:"reachability_edges_enabled"\`` field and `if req.ReachabilityEdgesEnabled != nil { if err := s.m.SetReachabilityEdgesEnabled(*req.ReachabilityEdgesEnabled); err != nil { writeErr(w, 500, err.Error()); return } }` block.

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./server/ -run TestReachabilityEdgesEnabledSettingsAPIRoundTrip -v`
Expected: PASS

- [ ] **Step 9: Write the failing agent-side wiring tests**

Mirror `agent/burn_signal_test.go` exactly (two tests: nil-var defaults false, var reflects a flipped closure), renamed for `ReachabilityEdgesEnabled`/`reachabilityEdgesEnabled`.

- [ ] **Step 10: Run tests to verify they fail**

Run: `go test ./agent/ -run TestReachabilityEdgesEnabled -v`
Expected: FAIL (undefined)

- [ ] **Step 11: Implement the agent-side variable + helper, and wire it in `server/assembly.go`**

`agent/reachability.go`:
```go
package agent

// ReachabilityEdgesEnabled reports whether report_finding's "reaches"
// parameter (and the resulting asset_edges data in graph_overview) is
// active. Assigned once at startup (server/assembly.go's wireTools); nil
// until then, which reachabilityEdgesEnabled treats as off.
var ReachabilityEdgesEnabled func() bool

func reachabilityEdgesEnabled() bool {
	return ReachabilityEdgesEnabled != nil && ReachabilityEdgesEnabled()
}
```

In `server/assembly.go`'s `wireTools`, next to `agent.BurnSignalEnabled`:
```go
agent.ReachabilityEdgesEnabled = func() bool { return pg.GetBool(settingReachabilityEdges, false) }
```

- [ ] **Step 12: Run tests to verify they pass**

Run: `go test ./agent/ -run TestReachabilityEdgesEnabled -v`
Expected: PASS

- [ ] **Step 13: Commit**

```bash
git add server/manager.go server/server.go server/assembly.go server/reachability_test.go agent/reachability.go agent/reachability_test.go
git commit -m "feat(server,agent): wire reachability_edges_enabled setting + settings API (default off)"
```

---

### Task 3: `report_finding`'s `reaches` parameter, evidence-gated

**Files:**
- Modify: `agent/tools.go` (`addFinding()`, `agent/tools.go:1249-1328`)
- Test: `agent/reachability_edge_test.go` (new)

**Interfaces:**
- Consumes: `db.AssetRelCanReach`, `ExplorationStore.AddAssetEdge`/`AssetEdges` (Task 1); `reachabilityEdgesEnabled()` (Task 2); `t.findingRefsFromHint(hintID int64, explicit []db.TrafficRef) ([]db.TrafficRef, error)` (`agent/finding_workflow.go:110`); `pidList(raw []json.RawMessage) []int64` (`agent/tools.go:261`); `FindingRecorder` interface (`agent/finding_recorder.go:11-13`).
- Produces: `addFinding()`'s schema gains a `reaches` property (array of integer asset ids); the handler rejects the whole call when `reaches` is non-empty but no evidence resolved, or when `reachabilityEdgesEnabled()` is false; on success, writes one `can_reach` edge per (src in `asset_ids`, dst in `reaches`) pair, skipping self-pairs. Used by Task 4 (schema gating) and Task 5 (planner visibility reads what this task writes).

- [ ] **Step 1: Write the failing tests**

Four cases in one test file, using a test-local stub recorder (mirroring `failingFindingRecorder` in `agent/finding_recorder_test.go:15-18`) so the test exercises `addFinding`'s own `reaches` logic without needing a real traffic-capture backend:

```go
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
    return &db.RecordedFinding{FindingID: 999, NodeID: 1000}, nil
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
        "asset_ids": []int64{srcID, dstID}, // dstID included deliberately: the (dstID,dstID) self-pair must be skipped
        "reaches":   []int64{dstID},
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./agent/ -run TestReportFindingReaches -v`
Expected: FAIL (`reaches` not recognized / no rejection / no edge written — a plain build error is also an acceptable first failure if the stub type references something not yet adjusted; confirm it's not a typo before moving on)

- [ ] **Step 3: Implement**

Add to `addFinding()`'s schema (`agent/tools.go:1250-1256`'s `obj(map[string]any{...}, ...)` call), alongside `asset_ids`:
```go
"reaches": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "optional: other asset ids this finding's asset(s) (asset_ids) can reach — e.g. via SSRF, a pivot, or a leaked credential's scope. Requires verified traffic_refs or evidence_hint_id on this same call: a reaches claim with no evidence is rejected outright, not silently dropped."},
```
Add `Reaches []json.RawMessage \`json:"reaches"\`` to the anonymous input struct.

After the existing `refs, err := t.findingRefsFromHint(pid(a.EvidenceHintID), a.TrafficRefs)` call and its error check, before building `input`, add the hard-reject check:
```go
if len(a.Reaches) > 0 {
    if !reachabilityEdgesEnabled() {
        return actool.Errorf("reachability edges are disabled; omit reaches, or ask an operator to enable reachability_edges_enabled"), nil
    }
    if len(refs) == 0 {
        return actool.Errorf("reaches requires verified evidence (traffic_refs or evidence_hint_id) on this same call — a reachability claim needs real proof, it cannot ride along on an unevidenced finding"), nil
    }
    if len(a.AssetIDs) == 0 {
        return actool.Errorf("reaches requires asset_ids as the source of the reachability edge"), nil
    }
}
```
(Note: when `findingTrafficBindingEnabled()` is false, the existing code at `agent/tools.go:1276-1278` already zeros `a.TrafficRefs`/`a.EvidenceHintID` before this point, so `refs` is naturally empty and the `len(refs) == 0` branch above fires — `reaches` is therefore correctly blocked whenever traffic binding itself is off, with no separate check needed for that setting.)

After the existing success path (`recorded, err := ...` succeeds, `err != nil` check passed), add the edge-writing loop — placed after `t.writes.Findings++` and before building `result`:
```go
if len(a.Reaches) > 0 {
    srcs, dsts := pidList(a.AssetIDs), pidList(a.Reaches)
    for _, src := range srcs {
        for _, dst := range dsts {
            if src <= 0 || dst <= 0 || src == dst {
                continue
            }
            _ = t.ts.AddAssetEdge(src, db.AssetRelCanReach, dst) // best-effort: the finding itself is already recorded and must not be undone by an edge-write hiccup
        }
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./agent/ -run TestReportFindingReaches -v`
Expected: PASS

- [ ] **Step 5: Run the whole `agent` package suite to confirm nothing else regressed**

Run: `go test ./agent/... -v`
Expected: PASS (or only the already-ledgered, pre-existing `TestGraphOverviewExpandsAssociatedCompanyScope` failure from prior workstreams — no new failures)

- [ ] **Step 6: Commit**

```bash
git add agent/tools.go agent/reachability_edge_test.go
git commit -m "feat(agent): report_finding gains an evidence-gated reaches parameter"
```

---

### Task 4: Schema gating — strip `reaches` when disabled

**Files:**
- Modify: `agent/finding_workflow.go` (`findingWorkflowTools`, `agent/finding_workflow.go:25-83`)
- Test: `agent/finding_workflow_reaches_test.go` (new)

**Interfaces:**
- Consumes: `reachabilityEdgesEnabled()` (Task 2); `findingWorkflowTools(agentKey string, tools []actool.CoreTool) ([]actool.CoreTool, string)`'s existing structure (two return points, `agent/finding_workflow.go:47,82`); `DecorateTool` (used elsewhere in the same file).
- Produces: a new unexported `stripReachesIfDisabled(tools []actool.CoreTool) []actool.CoreTool` helper, applied to `out` immediately before both of `findingWorkflowTools`'s existing `return` statements, so `report_finding`'s advertised schema never includes `reaches` while the setting is off — independently of whatever `findingTrafficBindingEnabled()` is doing in the same function (the two settings are checked separately; `reaches` additionally happens to require traffic binding at the handler level per Task 3, but the schema-level strip here is gated purely on `reachabilityEdgesEnabled()`).

- [ ] **Step 1: Write the failing test**

Mirror `TestFindingWorkflowSharedAssemblyAndSwitch`'s structure (`agent/finding_workflow_test.go:12-61`) but toggling `ReachabilityEdgesEnabled` instead of `FindingTrafficBindingEnabled`, checking the presence/absence of `reaches` in `report_finding`'s schema:

```go
func TestFindingWorkflowStripsReachesWhenDisabled(t *testing.T) {
    oldReach, oldResolve, oldAugment := ReachabilityEdgesEnabled, ToolResolve, ToolAugment
    t.Cleanup(func() { ReachabilityEdgesEnabled, ToolResolve, ToolAugment = oldReach, oldResolve, oldAugment })
    ToolAugment = nil
    ToolResolve = nil
    on := false
    ReachabilityEdgesEnabled = func() bool { return on }
    ts := NewToolSet(nil, "fixture")

    for _, enabled := range []bool{false, true, false} {
        on = enabled
        out, _, cleanup := AugmentTools(t.Context(), "worker", []actool.CoreTool{ts.addFinding()})
        defer cleanup()
        for _, tool := range out {
            if tool.Name() != "report_finding" {
                continue
            }
            raw, _ := json.Marshal(tool.InputSchema())
            var schema map[string]any
            json.Unmarshal(raw, &schema)
            props, _ := schema["properties"].(map[string]any)
            if (props["reaches"] != nil) != on {
                t.Fatalf("enabled=%v: reaches presence in schema = %v, want %v", on, props["reaches"] != nil, on)
            }
        }
    }
}
```

(Check `ToolAugment`/`ToolResolve`'s zero-value behavior against how `TestFindingWorkflowSharedAssemblyAndSwitch` already handles them — adjust the fixture setup to whatever makes `AugmentTools` actually invoke `findingWorkflowTools` on the given tool list; read that existing test once more alongside `agent/assembly.go`'s `AugmentTools` if the skeleton above doesn't compile cleanly against the real call shape.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./agent/ -run TestFindingWorkflowStripsReachesWhenDisabled -v`
Expected: FAIL (`reaches` present regardless of the setting, since nothing strips it yet)

- [ ] **Step 3: Implement**

```go
// stripReachesIfDisabled removes report_finding's "reaches" parameter when
// reachability_edges_enabled is off. Independent of findingTrafficBindingEnabled
// above -- the two settings are configured and checked separately, even though
// Task 3's handler additionally requires traffic binding to be on before a
// reaches claim can carry evidence.
func stripReachesIfDisabled(tools []actool.CoreTool) []actool.CoreTool {
	if reachabilityEdgesEnabled() {
		return tools
	}
	out := make([]actool.CoreTool, len(tools))
	for i, tool := range tools {
		if tool.Name() != "report_finding" {
			out[i] = tool
			continue
		}
		raw, _ := json.Marshal(tool.InputSchema())
		var schema map[string]any
		if json.Unmarshal(raw, &schema) == nil {
			if props, ok := schema["properties"].(map[string]any); ok {
				delete(props, "reaches")
			}
			tool = DecorateTool(tool, tool.Description(), schema)
		}
		out[i] = tool
	}
	return out
}
```
Change `findingWorkflowTools`'s two `return` statements (`agent/finding_workflow.go:47,82`) to `return stripReachesIfDisabled(out), ""` and `return stripReachesIfDisabled(out), guidance` respectively.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./agent/ -run TestFindingWorkflowStripsReachesWhenDisabled -v`
Expected: PASS

- [ ] **Step 5: Run the whole `agent` package suite to confirm nothing else regressed**

Run: `go test ./agent/... -v`
Expected: PASS (modulo the already-ledgered pre-existing failure)

- [ ] **Step 6: Commit**

```bash
git add agent/finding_workflow.go agent/finding_workflow_reaches_test.go
git commit -m "feat(agent): strip report_finding's reaches parameter when reachability_edges_enabled is off"
```

---

### Task 5: Planner visibility — `graphOverviewData()` exposes asset reachability

**Files:**
- Modify: `agent/tools.go` (`graphOverviewData()`, the edge-fetch region around `agent/tools.go:385-401`)
- Test: `agent/reachability_overview_test.go` (new)

**Interfaces:**
- Consumes: `reachabilityEdgesEnabled()` (Task 2); `ExplorationStore.AssetEdges(limit int) ([]db.AssetEdge, error)` (Task 1).
- Produces: `graphOverviewData()`'s output map gains a new top-level key, present only when the setting is on: `out["asset_reachability"] = []map[string]any{{"src_asset_id": ..., "rel": ..., "dst_asset_id": ...}, ...}`. Used by Task 6 (the planner prompt references this key by name).

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./agent/ -run TestGraphOverviewExposesAssetReachabilityOnlyWhenEnabled -v`
Expected: FAIL (no `asset_reachability` key exists yet)

- [ ] **Step 3: Implement**

In `graphOverviewData()`, near the existing `edges, _ := t.ts.Edges(5000)` block (`agent/tools.go:389`), add an independent, separately-gated block (placement anywhere after `out` is initialized; it does not need to interleave with the exploration-edges logic since it reads a different table):
```go
if reachabilityEdgesEnabled() {
    assetEdges, _ := t.ts.AssetEdges(0) // best-effort: a query error degrades to no reachability data, never blocks the round
    reach := make([]map[string]any, 0, len(assetEdges))
    for _, e := range assetEdges {
        reach = append(reach, map[string]any{"src_asset_id": e.SrcAssetID, "rel": e.Rel, "dst_asset_id": e.DstAssetID})
    }
    out["asset_reachability"] = reach
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./agent/ -run TestGraphOverviewExposesAssetReachabilityOnlyWhenEnabled -v`
Expected: PASS

- [ ] **Step 5: Run the whole `agent` package suite to confirm nothing else regressed**

Run: `go test ./agent/... -v`
Expected: PASS (modulo the already-ledgered pre-existing failure)

- [ ] **Step 6: Commit**

```bash
git add agent/tools.go agent/reachability_overview_test.go
git commit -m "feat(agent): expose asset reachability edges in graph_overview behind reachability_edges_enabled"
```

---

### Task 6: Planner prompt guidance

**Files:**
- Modify: `agent/planner.go` (`plannerDefaultTmpl`, near the burn-signal bullet the W1 workstream added after "Keep the routes diverse", `agent/planner.go:331-332`)
- Test: `agent/planner_prompt_test.go` (extend the existing file from the W1 workstream with a new test function; do not modify the existing `TestPlannerPromptDocumentsBurnSignal`)

**Interfaces:**
- Consumes: nothing new — a pure prompt-text change referencing the `asset_reachability` key Task 5 added to `graph_overview`'s output.
- Produces: nothing consumed by another task — this is the workstream's last deliverable.

- [ ] **Step 1: Write the failing test**

```go
func TestPlannerPromptDocumentsAssetReachability(t *testing.T) {
    if !strings.Contains(plannerDefaultTmpl, "asset_reachability") {
        t.Fatal("plannerDefaultTmpl does not mention asset_reachability")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./agent/ -run TestPlannerPromptDocumentsAssetReachability -v`
Expected: FAIL

- [ ] **Step 3: Add the prompt bullet**

Insert a new bullet near the "Keep the routes diverse" / burn-signal bullets (same section, same voice), conveying (word to match the surrounding prose, do not copy verbatim): *when reachability data is visible, an asset_reachability entry means a confirmed finding showed one asset can reach another (e.g. via SSRF, a pivot, or a leaked credential's scope) — treat the reached asset as a candidate new route the same way an uncovered entry surface would be treated under "keep the routes diverse": check whether it is already covered by an open/running/recent_done intent, and if not, consider dispatching an intent against it. This is not itself a new exploration goal; it only re-prioritizes where the existing goal is pursued next.* Remember: no backtick characters anywhere in this string (`plannerDefaultTmpl` is a Go raw-string literal; a literal backtick breaks the build — this exact mistake was made and fixed in the W1 workstream).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./agent/ -run TestPlannerPromptDocumentsAssetReachability -v`
Expected: PASS

- [ ] **Step 5: Run the whole `agent` package suite once more to confirm the prompt change broke nothing else**

Run: `go test ./agent/... -v`
Expected: PASS (modulo the already-ledgered pre-existing failure)

- [ ] **Step 6: Commit**

```bash
git add agent/planner.go agent/planner_prompt_test.go
git commit -m "feat(agent): document asset reachability edges in the planner's system prompt"
```
