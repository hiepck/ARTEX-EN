# Stale-Negative Revival Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a worker mark, when recording a fact, that it invalidates one or more earlier facts (`supersedes`), and surface that to the planner every round so a direction abandoned because of a now-overturned conclusion can be reconsidered.

**Architecture:** A new `exploration_edges` relation (`supersedes`, fact → fact), written by a new evidence-gated parameter on the existing `record_fact` tool, mirroring `report_finding`'s already-shipped `reaches` parameter pattern exactly (feature-flagged, hard-rejected without real justification, written best-effort after the primary write succeeds). Exposed to the planner under a new `fact_supersedes` key in `graph_overview`, computed from the exploration-edges data `graphOverviewData()` already loads for lineage — no new table, no new query method.

**Tech Stack:** Go, PostgreSQL, the existing `agent`/`db`/`server` packages.

**Spec:** `docs/superpowers/specs/2026-10-19-stale-negative-revival-design.md`.

## Global Constraints

- **Default off**, like every experimental graph feature this session (`burn_signal_enabled`, `reachability_edges_enabled`): new setting `fact_supersedes_enabled`, wired through `Manager`, `server/assembly.go`, and both the GET payload and PATCH handler of `/api/settings` from the start (the W1 lesson: a flag with no settings-API surface is unreachable except via raw SQL).
- **Evidence-gated, hard-reject, not silently dropped**: a `supersedes` claim on a fact whose own `confidence != "observed"` or whose `evidence` is empty is rejected outright, mirroring `reaches`' "a reachability claim needs real proof, it cannot ride along on an unevidenced finding."
- **No new table, no new `ExplorationStore` query method.** `db.RelSupersedes` is a new value added to `exploration_edges`'s existing `ck_edge_rel` CHECK constraint (a real schema migration — corrected during this spec's builder-check from an earlier, factually wrong "no migration needed" draft). The planner-facing exposure reuses the `edges` slice `graphOverviewData()` already loads once per call (`agent/tools.go:400`, `t.ts.Edges(5000)`) via `ExplorationStore.Link`/the existing `Edges(limit)` method — no `FactSupersedes`-style dedicated method.
- **Scope: fact → fact, this exploration only.** A `supersedes` target must be an existing `KindFact` node of the *same exploration* (validated with `GetNode`, matching `record_fact`'s own existing `intent_id` check in the same function — not `GetNodeWithSources`, which `add_intent`'s `parent_ids` uses for a deliberately different, cross-task-linked purpose). Not finding, not intent, not goal, not another task's fact.
- **`old_intent_id` is embedded directly in each `fact_supersedes` entry**, computed from the `factFrom` map `graphOverviewData()` already builds (`agent/tools.go:403-412`) — not left for the planner to find by searching the truncated `recent_done_intents` window, which would silently fail for a fact old enough to have scrolled out of it (the exact motivating scenario).
- **`fact_supersedes` is absent (not `[]`) from `graph_overview` when the flag is off**, matching `asset_reachability`'s exact degrade shape.
- **No backticks inside the new `plannerDefaultTmpl` bullet text** — it is itself a backtick-delimited Go raw string literal (`agent/planner.go`); a literal backtick breaks the build (a mistake already made and fixed once this session, in W1).

## Review Focus

- **A `supersedes` claim with `confidence=inferred` or empty `evidence`** must be hard-rejected with a clear error, not silently accepted or silently dropped.
- **A `supersedes` target that doesn't exist, or isn't a `KindFact` of this exploration** (a finding, an intent, a goal, a fact from an unrelated task) must be rejected with a clear error naming the bad id, mirroring `add_intent`'s `parent_ids` validation message style.
- **The feature-flag default is off**, and when off, `record_fact` must reject a non-empty `supersedes` outright (consistent with `reaches`' own gate) rather than silently ignoring the parameter.
- **`fact_supersedes` must be entirely absent (not `[]`) when the flag is off**, and when on but no supersedes edges exist yet, present as `[]` (not absent) — mirroring `asset_reachability`'s exact two-state behavior.
- **`old_intent_id` must be correct even when the superseded fact's producing intent would not appear in a truncated `recent_done_intents` window** — this is the plan's one deliberately non-obvious correctness requirement, directly motivated by the scenario that justified building this feature at all.

---

### Task 1: Schema migration, relation constant, and feature-flag plumbing

**Files:**
- Modify: `db/schema.sql` (immediately after the existing `ck_edge_rel`-recreate `DO $$` block, ~line 273)
- Modify: `db/constants.go` (the `Rel*` block, lines 33-39)
- Create: `agent/supersedes.go`
- Modify: `server/manager.go` (near `settingReachabilityEdges`/`ReachabilityEdgesEnabled`, ~lines 285, 532-541)
- Modify: `server/assembly.go` (near `agent.ReachabilityEdgesEnabled = ...`, ~line 215)
- Modify: `server/server.go` (the settings GET payload ~line 3339, the PATCH request struct ~line 3415, and the PATCH handler body ~line 3451)
- Test: `server/supersedes_test.go`

**Interfaces:**
- Consumes: `pg.GetBool(key, default)`/`pg.SetBool(key, on)` (existing `db.DB` methods, already used by every sibling setting); `trafficEvidenceServer(t)` (existing server-package test fixture, already used by `server/reachability_test.go`).
- Produces (consumed by Task 2): `db.RelSupersedes` (const, value `"supersedes"`); `agent.FactSupersedesEnabled func() bool` (package var) and the unexported `factSupersedesEnabled() bool` helper; `Manager.FactSupersedesEnabled() bool` / `Manager.SetFactSupersedesEnabled(on bool) error`. A migrated `ck_edge_rel` constraint that accepts `'supersedes'` (Task 2's edge-write test is the end-to-end proof this landed correctly).

- [ ] **Step 1: Write the failing tests**

Create `server/supersedes_test.go`, mirroring `server/reachability_test.go` exactly (read that file first — both tests below are its direct analog):

```go
package server

import (
	"encoding/json"
	"testing"
)

func TestFactSupersedesEnabledDefaultsOffAndPersists(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	clearSetting := func() { _, _ = m.pg.Exec(`DELETE FROM settings WHERE key=$1`, "fact_supersedes_enabled") }
	clearSetting()
	t.Cleanup(clearSetting)

	if m.FactSupersedesEnabled() {
		t.Fatal("FactSupersedesEnabled() default = true, want false")
	}
	if err := m.SetFactSupersedesEnabled(true); err != nil {
		t.Fatalf("SetFactSupersedesEnabled: %v", err)
	}
	if !m.FactSupersedesEnabled() {
		t.Fatal("FactSupersedesEnabled() after Set(true) = false, want true")
	}
}

func TestFactSupersedesEnabledSettingsAPIRoundTrip(t *testing.T) {
	s, _, request := trafficEvidenceServer(t)
	pg := s.m.pg
	clearSetting := func() { _, _ = pg.Exec(`DELETE FROM settings WHERE key=$1`, "fact_supersedes_enabled") }
	clearSetting()
	t.Cleanup(clearSetting)

	read := func() bool {
		t.Helper()
		r := request("GET", "/api/settings", "")
		if r.Code != 200 {
			t.Fatalf("GET /api/settings failed %d: %s", r.Code, r.Body)
		}
		var payload map[string]any
		if err := json.Unmarshal(r.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode the settings payload: %v", err)
		}
		v, ok := payload["fact_supersedes_enabled"]
		if !ok {
			t.Fatal("settings payload is missing fact_supersedes_enabled")
		}
		on, ok := v.(bool)
		if !ok {
			t.Fatalf("fact_supersedes_enabled is not a bool: %T(%v)", v, v)
		}
		return on
	}

	if on := read(); on {
		t.Fatal("GET /api/settings fact_supersedes_enabled default = true, want false")
	}
	if r := request("PUT", "/api/settings", `{"fact_supersedes_enabled":true}`); r.Code != 200 {
		t.Fatalf("PUT /api/settings fact_supersedes_enabled failed %d: %s", r.Code, r.Body)
	}
	if on := read(); !on {
		t.Fatal("GET /api/settings fact_supersedes_enabled after PATCH(true) = false, want true")
	}
	if !s.m.FactSupersedesEnabled() {
		t.Fatal("Manager.FactSupersedesEnabled() after PATCH(true) = false, want true")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/ -run TestFactSupersedes -v`
Expected: FAIL (`FactSupersedesEnabled`/`SetFactSupersedesEnabled` undefined, or the settings keys missing from the payload)

- [ ] **Step 3: Implement**

In `db/schema.sql`, immediately after the existing `ck_edge_rel`-recreate `DO $$...END $$;` block (~line 273):
```sql
ALTER TABLE exploration_edges DROP CONSTRAINT IF EXISTS ck_edge_rel;
ALTER TABLE exploration_edges ADD CONSTRAINT ck_edge_rel
    CHECK (rel IN ('spawns','derived_from','yields','proves','covers','supersedes'));
```

In `db/constants.go`'s `Rel*` block, add:
```go
RelSupersedes = "supersedes" // fact --supersedes--> fact: a later fact overturns an earlier one's conclusion (this exploration only)
```

Create `agent/supersedes.go`, mirroring `agent/reachability.go` exactly:
```go
package agent

// FactSupersedesEnabled reports whether record_fact's "supersedes" parameter
// (and the resulting fact_supersedes data in graph_overview) is active.
// Assigned once at startup (server/assembly.go's wireTools); nil until then,
// which factSupersedesEnabled treats as off.
var FactSupersedesEnabled func() bool

func factSupersedesEnabled() bool {
	return FactSupersedesEnabled != nil && FactSupersedesEnabled()
}
```

In `server/manager.go`, add a `settingFactSupersedes = "fact_supersedes_enabled"` constant next to `settingReachabilityEdges`, and `Manager.FactSupersedesEnabled()`/`Manager.SetFactSupersedesEnabled(on bool) error`, mirroring `ReachabilityEdgesEnabled`/`SetReachabilityEdgesEnabled` exactly (lines 532-541).

In `server/assembly.go`, add `agent.FactSupersedesEnabled = func() bool { return pg.GetBool(settingFactSupersedes, false) }` next to the existing `agent.ReachabilityEdgesEnabled = ...` line.

In `server/server.go`: add `"fact_supersedes_enabled": s.m.FactSupersedesEnabled(),` to the settings GET payload next to `reachability_edges_enabled`; add `FactSupersedesEnabled *bool \`json:"fact_supersedes_enabled"\`` to the PATCH request struct; add the corresponding `if req.FactSupersedesEnabled != nil { ... s.m.SetFactSupersedesEnabled(...) ... }` block to the PATCH handler, mirroring the existing `ReachabilityEdgesEnabled` block exactly.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./server/ -run TestFactSupersedes -v`
Expected: PASS (2/2)

- [ ] **Step 5: Commit**

```bash
git add db/schema.sql db/constants.go agent/supersedes.go server/manager.go server/assembly.go server/server.go server/supersedes_test.go
git commit -m "feat(db,server): add the supersedes exploration-edge relation + its setting"
```

---

### Task 2: `record_fact`'s `supersedes` parameter, graph_overview exposure, planner prompt

**Files:**
- Modify: `agent/tools.go` (`factItem` struct ~line 1371, `recordOneFact` ~line 1382, `recordFact()`'s schema/description ~lines 1418-1434, `graphOverviewData()` ~lines 400-412)
- Modify: `agent/planner.go` (`plannerDefaultTmpl`, after the existing `asset_reachability` bullet, ~line 333)
- Test: `agent/supersedes_edge_test.go`, `agent/supersedes_overview_test.go`

**Interfaces:**
- Consumes: `db.RelSupersedes`, `agent.FactSupersedesEnabled`/`factSupersedesEnabled()` (Task 1); `t.ts.GetNode(id)`, `t.ts.Link(from, rel, to)`, `t.ts.Edges(limit)` (pre-existing `ExplorationStore` methods, unchanged).
- Produces: nothing consumed by a later task — this is the plan's final task.

- [ ] **Step 1: Write the failing tests**

Create `agent/supersedes_edge_test.go`, mirroring `agent/reachability_edge_test.go`'s three tests exactly in shape (read that file first), substituting fact/fact semantics for finding/asset:

```go
package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestRecordFactSupersedesRequiresEvidence(t *testing.T) {
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })
	FactSupersedesEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes no evidence", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	oldFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "old blocking fact"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"summary": "new fact", "confidence": "inferred", "evidence": "",
		"supersedes": []int64{oldFactID},
	})
	res, err := ts.recordFact().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "observed") && !strings.Contains(res.Flatten(), "evidence") {
		t.Fatalf("expected an evidence-required rejection, got: %s", res.Flatten())
	}
	facts, _ := store.ListByKind(db.KindFact, 10)
	if len(facts) != 1 { // only oldFactID; the rejected new fact must not have been written
		t.Fatalf("got %d facts, want 1 (the rejected call must write nothing)", len(facts))
	}
}

func TestRecordFactSupersedesRequiresSettingOn(t *testing.T) {
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })
	FactSupersedesEnabled = func() bool { return false }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes setting off", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	oldFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "old blocking fact"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"summary": "new fact", "confidence": "observed", "evidence": "fresh session token works",
		"supersedes": []int64{oldFactID},
	})
	res, err := ts.recordFact().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "disabled") && !strings.Contains(res.Flatten(), "supersed") {
		t.Fatalf("expected a disabled-setting rejection, got: %s", res.Flatten())
	}
	facts, _ := store.ListByKind(db.KindFact, 10)
	if len(facts) != 1 {
		t.Fatalf("got %d facts, want 1 (the rejected call must write nothing)", len(facts))
	}
}

func TestRecordFactSupersedesRejectsNonFactTarget(t *testing.T) {
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })
	FactSupersedesEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes bad target", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	intentID, err := store.AddIntent(map[string]any{"summary": "an intent, not a fact"}, 5, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"summary": "new fact", "confidence": "observed", "evidence": "proof",
		"supersedes": []int64{intentID},
	})
	res, err := ts.recordFact().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Flatten(), "does not exist") && !strings.Contains(res.Flatten(), "fact") {
		t.Fatalf("expected a bad-target rejection naming the id, got: %s", res.Flatten())
	}
}

func TestRecordFactSupersedesWritesEdgeOnSuccess(t *testing.T) {
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })
	FactSupersedesEnabled = func() bool { return true }

	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes success", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)
	oldFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "old: 403 looks like an authz gate"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}

	ts := NewToolSet(store, "worker")
	ts.taskID = task.ID
	body, _ := json.Marshal(map[string]any{
		"summary": "new: the 403 was a wrong-credential-type artifact, a fresh session token works",
		"confidence": "observed", "evidence": "fresh session accessToken returned 200",
		"supersedes": []int64{oldFactID},
	})
	res, err := ts.recordFact().Call(t.Context(), json.RawMessage(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Flatten(), "fact recorded: ") {
		t.Fatalf("expected success, got: %s", res.Flatten())
	}

	edges, err := store.Edges(0)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range edges {
		if e.Rel == db.RelSupersedes && e.To == oldFactID {
			found = true
		}
	}
	if !found {
		t.Fatalf("no supersedes edge -> old fact %d found in %+v", oldFactID, edges)
	}
}
```

Create `agent/supersedes_overview_test.go`, mirroring `agent/reachability_overview_test.go`'s shape, proving the `old_intent_id` correctness requirement from Review Focus directly (this is the test that would have caught the builder-check's original graph_overview-placement defect):

```go
package agent

import (
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestGraphOverviewExposesFactSupersedesWithOldIntentID(t *testing.T) {
	d := testDB(t)
	defer d.Close()
	task, err := d.CreateTask("supersedes overview", "fixture", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteTask(task.ID)
	store := d.Exploration(task.ExplorationID)

	intentID, err := store.AddIntent(map[string]any{"summary": "probe getFollowerAssets"}, 5, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	oldFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "403, looks like a seller-state gate"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Link(intentID, db.RelYields, oldFactID); err != nil {
		t.Fatal(err)
	}
	// Simulate the intent having long since finished and scrolled out of any
	// recent-window list -- the overview must still resolve old_intent_id
	// correctly, since it is computed from factFrom, not from recent_done_intents.
	if err := store.SetIntentState(intentID, "done"); err != nil {
		t.Fatal(err)
	}

	tools := NewToolSet(store, "planner")
	old := FactSupersedesEnabled
	t.Cleanup(func() { FactSupersedesEnabled = old })

	FactSupersedesEnabled = func() bool { return false }
	off := tools.graphOverviewData()
	if _, has := off["fact_supersedes"]; has {
		t.Fatalf("fact_supersedes present while disabled: %+v", off["fact_supersedes"])
	}

	FactSupersedesEnabled = func() bool { return true }
	onEmpty := tools.graphOverviewData()
	if sup, ok := onEmpty["fact_supersedes"].([]map[string]any); !ok || len(sup) != 0 {
		t.Fatalf("fact_supersedes with no edges yet = %#v, want an empty slice, not absent/nil", onEmpty["fact_supersedes"])
	}

	newFactID, err := store.AddNode(db.KindFact, map[string]any{"summary": "fresh session token works, it was a credential-type issue"}, 5, "confirmed", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Link(newFactID, db.RelSupersedes, oldFactID); err != nil {
		t.Fatal(err)
	}

	on := tools.graphOverviewData()
	sup, ok := on["fact_supersedes"].([]map[string]any)
	if !ok || len(sup) != 1 {
		t.Fatalf("fact_supersedes = %#v, want exactly 1 entry", on["fact_supersedes"])
	}
	if sup[0]["new_fact_id"] != newFactID || sup[0]["old_fact_id"] != oldFactID {
		t.Fatalf("entry = %+v, want new=%d old=%d", sup[0], newFactID, oldFactID)
	}
	if sup[0]["old_intent_id"] != intentID {
		t.Fatalf("old_intent_id = %v, want %d (must resolve via factFrom, not a windowed intent list)", sup[0]["old_intent_id"], intentID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./agent/ -run "TestRecordFactSupersedes|TestGraphOverviewExposesFactSupersedes" -v`
Expected: FAIL (`supersedes` field unrecognized / no rejection occurs / `fact_supersedes` never appears — compile should succeed since `FactSupersedesEnabled` already exists from Task 1, but the behavior is missing)

- [ ] **Step 3: Implement**

In `agent/tools.go`'s `factItem` struct (~line 1371), add:
```go
Supersedes []json.RawMessage `json:"supersedes"`
```

In `recordFact()`'s JSON schema (~lines 1426-1434), add:
```go
"supersedes": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "optional: ids of earlier facts this new fact invalidates or overturns (e.g. \"fact #1948 said this endpoint has an authz gate; this fact shows the 403 was actually a wrong-credential-type artifact\"). Requires confidence=observed and a non-empty evidence on this same fact -- overturning a settled conclusion needs real proof, not a guess."},
```

In `recordOneFact` (~line 1382), add the gating block after the existing `summary` emptiness check and before the node is written, in this exact order (mirroring `addFinding`'s `reaches` gate order):
1. `if len(it.Supersedes) > 0 && !factSupersedesEnabled() { return 0, fmt.Errorf("fact supersession is disabled; omit supersedes, or ask an operator to enable fact_supersedes_enabled") }`
2. `if len(it.Supersedes) > 0 && (strings.TrimSpace(it.Confidence) != "observed" || strings.TrimSpace(it.Evidence) == "") { return 0, fmt.Errorf("supersedes requires confidence=observed and a non-empty evidence on this same fact -- a claim to overturn an earlier conclusion needs real proof, not a guess") }`
3. Validate each id in `pidList(it.Supersedes)` with `t.ts.GetNode(id)`: must be non-nil and `Kind == db.KindFact`, else `return 0, fmt.Errorf("supersedes id %d does not exist as a fact of this task", id)` (mirroring `addOneIntent`'s `parent_id` error wording).

After the fact node is successfully written and its `intent --yields--> fact` link is made, add the best-effort edge write (mirroring `addFinding`'s reaches-writing loop's "must not undo the primary write" rationale):
```go
for _, oldID := range pidList(it.Supersedes) {
	_ = t.ts.Link(id, db.RelSupersedes, oldID)
}
```

In `graphOverviewData()` (`agent/tools.go`), immediately after the `parentsOf`/`yieldsOf`/`factFrom` loop ends (~line 412, right after the closing brace of the `for _, e := range edges` loop that builds `factFrom`), add:
```go
// fact_supersedes: a later fact that overturned an earlier one's conclusion.
// old_intent_id is looked up from factFrom (built just above), not from the
// truncated recent_done_intents window -- the old fact may be long gone from
// that window by the time something invalidates it.
if factSupersedesEnabled() {
	sup := make([]map[string]any, 0)
	for _, e := range edges {
		if e.Rel != db.RelSupersedes {
			continue
		}
		entry := map[string]any{"new_fact_id": e.From, "old_fact_id": e.To}
		if oldIntent := factFrom[e.To]; oldIntent > 0 {
			entry["old_intent_id"] = oldIntent
		}
		sup = append(sup, entry)
	}
	out["fact_supersedes"] = sup
}
```

In `agent/planner.go`'s `plannerDefaultTmpl`, immediately after the existing `asset_reachability` bullet (~line 333), add (no backticks):
```
   - **Treat a superseded fact as a revival signal (when visible)**: when an admin has turned the setting on, fact_supersedes above lists entries {new_fact_id, old_fact_id, old_intent_id} -- a later fact that has overturned an earlier one's conclusion, and (when present) the intent that earlier fact came from. If old_intent_id is done/blocked/exhausted because of that now-overturned conclusion and the direction is still in scope and worth pursuing, dispatch a new intent with add_intent(parent_ids=[new_fact_id], ...) explaining what changed -- look it up with node_detail if it is not already in open/running/recent_done_intents. This does not reopen the old intent or create a new goal on its own -- it only flags a direction worth a fresh look. Do nothing extra when fact_supersedes is absent or empty.
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./agent/ -run "TestRecordFactSupersedes|TestGraphOverviewExposesFactSupersedes" -v`
Expected: PASS (5/5)

- [ ] **Step 5: Run the whole `agent`, `db`, and `server` package suites to confirm nothing else regressed**

Run: `go test ./agent/... ./db/... ./server/... -p 1 -v`
Expected: PASS (or only the already-documented pre-existing baseline flakiness from this session's prior workstreams — no new failures)

- [ ] **Step 6: Commit**

```bash
git add agent/tools.go agent/planner.go agent/supersedes_edge_test.go agent/supersedes_overview_test.go
git commit -m "feat(agent): add record_fact's evidence-gated supersedes parameter"
```
