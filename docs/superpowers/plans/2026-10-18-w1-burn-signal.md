# W1: Planner Burn-Signal Visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the planner per-intent resource-burn visibility (tool calls, tokens, elapsed seconds since claimed) for every open/running intent in `graph_overview`, behind a new opt-in setting, so the planner can act on its existing `kill_work`/`steer_work` callbacks before a hard `max_turns`/`run_seconds` cap trips — not after.

**Architecture:** A new read-side aggregation (`ExplorationStore.BurnStatsByIntent`) computes burn purely from the existing `activity` table (no schema change). It is threaded through `graphOverviewData()`/`compactIntents()` into the planner-visible JSON, gated behind a new `burn_signal_enabled` setting (default off, same `Manager.XEnabled()/SetX()` + package-level function-variable pattern as `noa_compaction`/`agent_traffic_binding`). The planner's system prompt is updated to document the new field and connect it to the kill_work/steer_work decision it already makes qualitatively.

**Tech Stack:** Go, PostgreSQL (`database/sql` via the `pgx/v5/stdlib` driver), the existing `agent`/`db`/`server` packages.

**Spec:** `docs/superpowers/specs/2026-10-17-core-engine-finding-capability-design.md` (workstream W1).

## Global Constraints

- **No breaking schema change**: this workstream is pure read-side aggregation over the existing `activity` table — no `ALTER TABLE`, no new table. If a task seems to need one, that's a signal the plan or spec is wrong, not a passing deviation.
- **Opt-in, default off**: every behavior change in this plan must be reachable only through the new `burn_signal_enabled` setting (default `false`). An operator who does nothing must see zero behavior change — no new field in `graph_overview`, no extra DB query, nothing.
- **Follow the `noa_compaction`/`agent_traffic_binding` feature-flag template exactly**: a `settingX` constant in `server/manager.go`, a `Manager.XEnabled()/SetX()` pair backed by `pg.GetBool`/`pg.SetBool`, and (because the consuming code lives in package `agent`, which cannot import `server`) a package-level function variable in `agent` (mirroring `agent.FindingTrafficBindingEnabled`, `agent/finding_workflow.go:16-20`) assigned once at startup in `server/assembly.go`'s `wireTools`.
- **House style for array-parameter queries**: this codebase does not pass a Go slice directly into `= ANY($1)`; it builds `$N` placeholders manually and passes `args ...any` (see `db/assets.go:1368-1379`, `GetByIDs`). Follow that exact pattern, not a driver-specific array binding.
- **Burn is per-intent, not per-task**: the aggregation is keyed by `node_id` (the intent), matching `TokenStatsBySession`'s `'intent:'||node_id` session-key convention (`db/exploration.go:1449-1479`) — do not conflate it with task- or exploration-wide totals.

## Review Focus

- **An intent with zero activity rows (just claimed, or never claimed)** → `BurnStatsByIntent` must not return a bogus zero-value entry for it (a zero `ElapsedSeconds` would look identical to "no burn yet" vs. "we have no data" otherwise) — the method must omit it from the returned map entirely, and `compactIntents` must only attach a `burn` field when the map actually has an entry for that intent id. (Task 1, Task 3)
- **The flag is off (the default)** → `graphOverviewData()` must not run the new query at all and must not add a `burn` key anywhere in its output — not just an empty/zero one. (Task 3)
- **Concurrent workers writing activity rows while the planner reads burn stats mid-round** → the aggregation must be a single `SELECT ... GROUP BY`, not a read-modify-write, so it can't observe a torn/inconsistent intermediate state; confirm no part of this plan writes to `activity` as a side effect of reading burn. (Task 1)
- **An intent id requested that belongs to a *different* exploration** (a stale id, a copy-paste bug in the caller) → the query must filter by `exploration_id` as well as `node_id`, the same cross-exploration isolation `TestTokenStatsBySessionUsesCompleteIntentHistory` already exercises for tokens (`db/exploration_tokens_test.go:28-38`) — a burn-stats test must assert the same isolation, not just assume it. (Task 1)
- **The planner prompt change silently breaking existing planner behavior tests** — `plannerDefaultTmpl` is a large string constant with other tests potentially asserting on its exact content; the new bullet must be additive (new text only), not a rewrite of an existing bullet, and the full `agent` package test suite must stay green after the change. (Task 4)

---

### Task 1: `BurnStats` type + `ExplorationStore.BurnStatsByIntent`

**Files:**
- Modify: `db/exploration.go` (add the type and method near `TokenStatsBySession`, `db/exploration.go:1449`)
- Test: `db/burn_stats_test.go` (new)

**Interfaces:**
- Consumes: `ExplorationStore.AddIntent(payload map[string]any, priority int, anchors []int64, origin string) (int64, error)` (`db/exploration.go:220`), `ExplorationStore.AppendActivity(a Activity) (int64, error)` (`db/exploration.go:1209`), `testDSN(t *testing.T) string` (`db/db_test.go:56`), `d.CreateExploration(description, goal string) (int64, error)` (`db/exploration.go:178`).
- Produces: `type BurnStats struct { ToolCalls int; InputTokens int; OutputTokens int; ElapsedSeconds float64 }` and `func (s *ExplorationStore) BurnStatsByIntent(ids []int64) (map[int64]BurnStats, error)` — returns one entry per id that has at least one `activity` row, omitting ids with none; `len(ids)==0` returns an empty map, no query. Used by Task 3.

- [ ] **Step 1: Write the failing test**

```go
package db

import "testing"

func TestBurnStatsByIntentAggregatesAndIsolates(t *testing.T) {
    d, err := Open(testDSN(t))
    if err != nil {
        t.Skipf("postgres unavailable (%v) - skipping", err)
    }
    defer d.Close()

    expID, err := d.CreateExploration("burn stats", "aggregate per-intent burn")
    if err != nil {
        t.Fatal(err)
    }
    defer d.Exec(`DELETE FROM explorations WHERE id=$1`, expID)
    store := d.Exploration(expID)

    busyID, err := store.AddIntent(map[string]any{"summary": "busy intent"}, 1, nil, "planner")
    if err != nil {
        t.Fatal(err)
    }
    idleID, err := store.AddIntent(map[string]any{"summary": "idle intent, no activity"}, 1, nil, "planner")
    if err != nil {
        t.Fatal(err)
    }

    otherExpID, err := d.CreateExploration("burn stats other", "foreign intent must not match")
    if err != nil {
        t.Fatal(err)
    }
    defer d.Exec(`DELETE FROM explorations WHERE id=$1`, otherExpID)
    foreignID, err := d.Exploration(otherExpID).AddIntent(map[string]any{"summary": "foreign"}, 1, nil, "planner")
    if err != nil {
        t.Fatal(err)
    }
    // Give the foreign intent its own activity so a leak would be visible, not just absent-by-coincidence.
    if _, err := d.Exploration(otherExpID).AppendActivity(Activity{NodeID: &foreignID, Worker: "work#1", Kind: "tool_use"}); err != nil {
        t.Fatal(err)
    }

    in, out := 100, 50
    appendToolUse := func() {
        t.Helper()
        if _, err := store.AppendActivity(Activity{NodeID: &busyID, Worker: "work#1", Kind: "tool_use"}); err != nil {
            t.Fatal(err)
        }
    }
    appendToolUse()
    appendToolUse()
    if _, err := store.AppendActivity(Activity{NodeID: &busyID, Worker: "work#1", Kind: "result", InputTokens: &in, OutputTokens: &out}); err != nil {
        t.Fatal(err)
    }
    time.Sleep(1100 * time.Millisecond) // give ElapsedSeconds something nonzero to observe

    got, err := store.BurnStatsByIntent([]int64{busyID, idleID, foreignID})
    if err != nil {
        t.Fatal(err)
    }
    if len(got) != 1 {
        t.Fatalf("got %d entries, want exactly 1 (busy only): %+v", len(got), got)
    }
    b, ok := got[busyID]
    if !ok {
        t.Fatalf("busy intent missing from result: %+v", got)
    }
    if b.ToolCalls != 3 { // 2 tool_use + 1 result (result also matches the IN-clause filter, see Step 3)
        t.Fatalf("ToolCalls = %d, want 3", b.ToolCalls)
    }
    if b.InputTokens != 100 || b.OutputTokens != 50 {
        t.Fatalf("tokens = %d/%d, want 100/50", b.InputTokens, b.OutputTokens)
    }
    if b.ElapsedSeconds < 1.0 {
        t.Fatalf("ElapsedSeconds = %v, want >= 1.0 after the sleep", b.ElapsedSeconds)
    }
    if _, ok := got[idleID]; ok {
        t.Fatalf("idle intent (no activity) must be absent, not a zero-value entry: %+v", got[idleID])
    }
    if _, ok := got[foreignID]; ok {
        t.Fatalf("foreign-exploration intent must never appear: %+v", got[foreignID])
    }

    empty, err := store.BurnStatsByIntent(nil)
    if err != nil || len(empty) != 0 {
        t.Fatalf("BurnStatsByIntent(nil) = %+v, %v; want empty map, nil error", empty, err)
    }
}
```

Add `"time"` to the test file's imports.

Before finalizing the `ToolCalls` assertion, decide (this is the one open judgment call in this task): should the tool-call count include `kind='result'` rows or only `kind IN ('tool_use','tool_result')`? The test above assumes `tool_use`+`tool_result`+`result` all count (3 total for 2 tool_use + 1 result) to match the literal `kind IN ('tool_use','tool_result')` filter plan text — if you judge `result` should NOT count as a "tool call" (it's the turn's final summary row, not a tool invocation), adjust the test's expected count to 2 and the implementation's filter accordingly, and note the choice in a one-line code comment. Either is defensible; pick one and be internally consistent between the test and the implementation.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestBurnStatsByIntentAggregatesAndIsolates -v`
Expected: FAIL (`store.BurnStatsByIntent` undefined / build error)

- [ ] **Step 3: Implement `BurnStats` and `BurnStatsByIntent`**

```go
type BurnStats struct {
    ToolCalls      int
    InputTokens    int
    OutputTokens   int
    ElapsedSeconds float64
}

func (s *ExplorationStore) BurnStatsByIntent(ids []int64) (map[int64]BurnStats, error) {
    if len(ids) == 0 {
        return map[int64]BurnStats{}, nil
    }
    ...
}
```

Build the query following `db/assets.go:1368-1379`'s placeholder-building idiom (`$1` = `s.expID`, `$2..$N+1` = the ids, `node_id IN (...)`, not `= ANY($1)`), `GROUP BY node_id`, filtering `exploration_id=$1` (the isolation the Review Focus and the test's `foreignID` case require). Use `EXTRACT(EPOCH FROM (now() - MIN(created_at)))` for `ElapsedSeconds`, `COALESCE(SUM(...) FILTER (WHERE kind='result'), 0)` for the token sums (mirroring `TokenStatsBySession`'s token-summing pattern, `db/exploration.go:1457-1458`), and `COUNT(*) FILTER (WHERE kind IN (...))` for `ToolCalls` per the judgment call made in Step 1. Only ids present in the result rows go into the returned map — do not pre-seed zero entries for the requested ids.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestBurnStatsByIntentAggregatesAndIsolates -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add db/exploration.go db/burn_stats_test.go
git commit -m "feat(db): add BurnStats + ExplorationStore.BurnStatsByIntent"
```

---

### Task 2: Feature-flag plumbing (`burn_signal_enabled`)

**Files:**
- Modify: `server/manager.go` (add the setting constant + getter/setter, near `settingNoaCompaction`, `server/manager.go:277` and `NoaCompactionEnabled`/`SetNoaCompaction`, `server/manager.go:498-505`)
- Modify: `server/assembly.go` (wire the package variable inside `wireTools`, next to `agent.FindingTrafficBindingEnabled = ...` at `server/assembly.go:213`)
- Create: `agent/burn_signal.go` (the package-level variable + nil-safe accessor, mirroring `agent/finding_workflow.go:16-20`)
- Test: `server/burn_signal_test.go` (new), `agent/burn_signal_test.go` (new)

**Interfaces:**
- Consumes: `(*db.DB).GetBool(key string, def bool) bool` / `SetBool(key string, val bool) error` (`db/settings.go:50,59`), `NewManager(dir, proxyAddr string) (*Manager, error)` (`server/manager.go:344`).
- Produces: `Manager.BurnSignalEnabled() bool` / `Manager.SetBurnSignalEnabled(on bool) error` (server-side setting access); `agent.BurnSignalEnabled func() bool` (the package variable Task 3's `graphOverviewData()` change will call through a small unexported `burnSignalEnabled() bool` helper, exactly like `findingTrafficBindingEnabled()`, `agent/finding_workflow.go:18-20`). No consumer of the agent-side variable exists yet — that's Task 3.

- [ ] **Step 1: Write the failing server-side test**

```go
package server

import "testing"

func TestBurnSignalEnabledDefaultsOffAndPersists(t *testing.T) {
    m, err := NewManager(t.TempDir(), "")
    if err != nil {
        t.Skipf("postgres unavailable (%v) — skipping", err)
    }
    defer m.Close()

    if m.BurnSignalEnabled() {
        t.Fatal("BurnSignalEnabled() default = true, want false")
    }
    if err := m.SetBurnSignalEnabled(true); err != nil {
        t.Fatalf("SetBurnSignalEnabled: %v", err)
    }
    if !m.BurnSignalEnabled() {
        t.Fatal("BurnSignalEnabled() after SetBurnSignalEnabled(true) = false, want true")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestBurnSignalEnabledDefaultsOffAndPersists -v`
Expected: FAIL (`m.BurnSignalEnabled` undefined)

- [ ] **Step 3: Implement the server-side setting**

In `server/manager.go`, add `settingBurnSignalEnabled = "burn_signal_enabled"` next to `settingNoaCompaction`, and:

```go
// BurnSignalEnabled reports whether per-intent resource-burn visibility is
// exposed to the planner in graph_overview (off by default; settings.burn_signal_enabled).
func (m *Manager) BurnSignalEnabled() bool {
    return m.pg.GetBool(settingBurnSignalEnabled, false)
}

// SetBurnSignalEnabled persists the burn-signal toggle.
func (m *Manager) SetBurnSignalEnabled(on bool) error {
    return m.pg.SetBool(settingBurnSignalEnabled, on)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/ -run TestBurnSignalEnabledDefaultsOffAndPersists -v`
Expected: PASS

- [ ] **Step 5: Write the failing agent-side wiring test**

```go
package agent

import "testing"

func TestBurnSignalEnabledDefaultsFalseWhenUnset(t *testing.T) {
    old := BurnSignalEnabled
    t.Cleanup(func() { BurnSignalEnabled = old })
    BurnSignalEnabled = nil
    if burnSignalEnabled() {
        t.Fatal("burnSignalEnabled() with nil var = true, want false")
    }
}

func TestBurnSignalEnabledReflectsVar(t *testing.T) {
    old := BurnSignalEnabled
    t.Cleanup(func() { BurnSignalEnabled = old })
    on := false
    BurnSignalEnabled = func() bool { return on }
    if burnSignalEnabled() {
        t.Fatal("burnSignalEnabled() = true before flipping on")
    }
    on = true
    if !burnSignalEnabled() {
        t.Fatal("burnSignalEnabled() = false after flipping on")
    }
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./agent/ -run TestBurnSignalEnabled -v`
Expected: FAIL (`BurnSignalEnabled`/`burnSignalEnabled` undefined)

- [ ] **Step 7: Implement the agent-side variable + helper, and wire it in `server/assembly.go`**

`agent/burn_signal.go`:
```go
package agent

// BurnSignalEnabled reports whether per-intent resource-burn visibility
// should be computed and exposed in graph_overview. Assigned once at
// startup (server/assembly.go's wireTools); nil until then, which
// burnSignalEnabled treats as off.
var BurnSignalEnabled func() bool

func burnSignalEnabled() bool {
    return BurnSignalEnabled != nil && BurnSignalEnabled()
}
```

In `server/assembly.go`'s `wireTools` (`server/assembly.go:213`), add directly below the `FindingTrafficBindingEnabled` line:
```go
agent.BurnSignalEnabled = func() bool { return pg.GetBool(settingBurnSignalEnabled, false) }
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./agent/ -run TestBurnSignalEnabled -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add server/manager.go server/assembly.go server/burn_signal_test.go agent/burn_signal.go agent/burn_signal_test.go
git commit -m "feat(server,agent): wire burn_signal_enabled setting (default off)"
```

---

### Task 3: Thread burn stats into `graphOverviewData`/`compactIntents`, gated

**Files:**
- Modify: `agent/tools.go` (`compactIntents`, `agent/tools.go:20-49`; `graphOverviewData`, `agent/tools.go:354-430` region where `fr`/`running` are built and passed to `compactIntents`)
- Test: `agent/burn_signal_wiring_test.go` (new)

**Interfaces:**
- Consumes: `db.BurnStats` + `ExplorationStore.BurnStatsByIntent(ids []int64) (map[int64]db.BurnStats, error)` (Task 1); `burnSignalEnabled() bool` (Task 2, unexported in package `agent` — callable directly from `agent/tools.go` since it's the same package); `testDB(t *testing.T) *db.DB` (`agent/insert_assets_test.go:13`); `NewToolSet(ts *db.ExplorationStore, worker string) *ToolSet` (`agent/tools.go:163`).
- Produces: `compactIntents(ns []*db.Node, parentsOf, yieldsOf map[int64][]int64, burn map[int64]db.BurnStats) []map[string]any` (signature change — both call sites in `graphOverviewData` must be updated in the same commit). Each output entry gains an optional `"burn": map[string]any{"tool_calls": int, "tokens": int, "elapsed_s": float64}` key, present only when `burn[n.ID]` has an entry. Used by Task 4 (prompt references this field by name).

- [ ] **Step 1: Write the failing test**

```go
package agent

import (
    "testing"

    "github.com/Autumn-27/artex/db"
)

func TestGraphOverviewExposesBurnOnlyWhenEnabled(t *testing.T) {
    d := testDB(t)
    defer d.Close()

    task, err := d.CreateTaskWithOptions("burn wiring", "expose burn to the planner", db.TaskCreateOptions{})
    if err != nil {
        t.Fatal(err)
    }
    defer d.DeleteTask(task.ID)

    store := d.Exploration(task.ExplorationID)
    intentID, err := store.AddIntent(map[string]any{"summary": "busy intent"}, 1, nil, "planner")
    if err != nil {
        t.Fatal(err)
    }
    if ok, err := store.ClaimIntent(intentID, "work#1"); err != nil || !ok {
        t.Fatalf("claim intent: ok=%v err=%v", ok, err)
    }
    if _, err := store.AppendActivity(db.Activity{NodeID: &intentID, Worker: "work#1", Kind: "tool_use"}); err != nil {
        t.Fatal(err)
    }

    tools := NewToolSet(store, "planner")

    old := BurnSignalEnabled
    t.Cleanup(func() { BurnSignalEnabled = old })

    BurnSignalEnabled = func() bool { return false }
    off := tools.graphOverviewData()
    for _, entry := range off["running_intents"].([]map[string]any) {
        if _, has := entry["burn"]; has {
            t.Fatalf("burn field present while BurnSignalEnabled is off: %+v", entry)
        }
    }

    BurnSignalEnabled = func() bool { return true }
    on := tools.graphOverviewData()
    running, _ := on["running_intents"].([]map[string]any)
    var found bool
    for _, entry := range running {
        if entry["id"] != intentID {
            continue
        }
        found = true
        burn, ok := entry["burn"].(map[string]any)
        if !ok {
            t.Fatalf("intent %d missing burn field while enabled: %+v", intentID, entry)
        }
        if tc, _ := burn["tool_calls"].(int); tc < 1 {
            t.Fatalf("burn.tool_calls = %v, want >= 1", burn["tool_calls"])
        }
    }
    if !found {
        t.Fatalf("intent %d not found in running_intents: %+v", intentID, running)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./agent/ -run TestGraphOverviewExposesBurnOnlyWhenEnabled -v`
Expected: FAIL (compile error on `compactIntents` call-site arity once Step 3 starts, or a missing-field assertion failure if you stub the signature first — either way, confirm it fails for the right reason before moving on, not a typo)

- [ ] **Step 3: Implement the wiring**

Change `compactIntents`'s signature to accept `burn map[int64]db.BurnStats` as a 4th parameter; inside the per-node loop, after the existing `parents`/`yields` attachment, add:
```go
if b, ok := burn[n.ID]; ok {
    m["burn"] = map[string]any{"tool_calls": b.ToolCalls, "tokens": b.InputTokens + b.OutputTokens, "elapsed_s": b.ElapsedSeconds}
}
```
In `graphOverviewData()`, after `fr` (open intents) and `running` are both built (`agent/tools.go`, the region around the existing `out["open_intents"] = compactIntents(fr, parentsOf, yieldsOf)` and `out["running_intents"] = compactIntents(running, parentsOf, yieldsOf)` calls — note these two calls are NOT adjacent in the current code; `running` is only fully populated after the loop that follows the `fr` assignment, so compute the combined id list and the single `BurnStatsByIntent` call once both slices exist, then update both call sites to pass it):
```go
var burn map[int64]db.BurnStats
if burnSignalEnabled() {
    ids := make([]int64, 0, len(fr)+len(running))
    for _, n := range fr {
        ids = append(ids, n.ID)
    }
    for _, n := range running {
        ids = append(ids, n.ID)
    }
    burn, _ = t.ts.BurnStatsByIntent(ids) // best-effort: a query error degrades to no burn data, never blocks the round
}
out["open_intents"] = compactIntents(fr, parentsOf, yieldsOf, burn)
...
out["running_intents"] = compactIntents(running, parentsOf, yieldsOf, burn)
```
A nil `burn` map is safe: `burn[n.ID]` on a nil map returns the zero value and `ok=false`, so no `"burn"` key is ever attached when the flag is off — matching the Review Focus requirement without an extra branch inside `compactIntents` itself.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./agent/ -run TestGraphOverviewExposesBurnOnlyWhenEnabled -v`
Expected: PASS

- [ ] **Step 5: Run the whole `agent` package suite to confirm nothing else regressed**

Run: `go test ./agent/... -v`
Expected: PASS (or only pre-existing, unrelated skips/failures already known from prior work — no new failures caused by the `compactIntents` signature change; grep the test output for every other call site of `compactIntents` if any exist beyond `graphOverviewData` and confirm they were updated too)

- [ ] **Step 6: Commit**

```bash
git add agent/tools.go agent/burn_signal_wiring_test.go
git commit -m "feat(agent): expose per-intent burn stats in graph_overview behind burn_signal_enabled"
```

---

### Task 4: Planner prompt guidance

**Files:**
- Modify: `agent/planner.go` (`plannerDefaultTmpl`, `agent/planner.go:298`, specifically the "Decide which new directions to add" section around the existing "Depth beats coverage" / "Keep the routes diverse" bullets, `agent/planner.go:330-331`)
- Test: `agent/planner_prompt_test.go` (new)

**Interfaces:**
- Consumes: nothing new — this is a pure prompt-text change referencing the `burn` field Task 3 added to `graph_overview`'s `running_intents`/`open_intents` entries.
- Produces: nothing consumed by another task — this is the workstream's last deliverable.

- [ ] **Step 1: Write the failing test**

```go
package agent

import (
    "strings"
    "testing"
)

func TestPlannerPromptDocumentsBurnSignal(t *testing.T) {
    if !strings.Contains(plannerDefaultTmpl, "burn") {
        t.Fatal("plannerDefaultTmpl does not mention the burn field")
    }
    if !strings.Contains(plannerDefaultTmpl, "kill_work") || !strings.Contains(plannerDefaultTmpl, "steer_work") {
        t.Fatal("plannerDefaultTmpl's burn guidance does not connect it to kill_work/steer_work")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./agent/ -run TestPlannerPromptDocumentsBurnSignal -v`
Expected: FAIL (`plannerDefaultTmpl` doesn't yet mention "burn")

- [ ] **Step 3: Add the prompt bullet**

Insert a new bullet immediately after the existing "Keep the routes diverse" bullet (`agent/planner.go:331`), in the same voice and bullet style as its neighbors (bold lead term, then explanation). Content to convey (word it to match the surrounding prose style, not copied verbatim from here): *when `burn_signal_enabled` is on, each running/open intent in the state above carries a `burn` figure (tool calls / tokens / elapsed seconds since claimed); treat a high burn with no new facts/findings from that intent as a signal to act now rather than wait for its `max_turns`/`run_seconds` cap — use `get_worker_output` to look at what it's actually doing, then `steer_work` for a correctable direction or `kill_work` to cut losses on one that's clearly off track. As a rough, non-binding reference point: a running intent well past ~40 tool calls or ~5 minutes with no new fact/finding is usually worth a look, not an automatic kill — judge by what the trace actually shows, not the numbers alone.* Keep it to one bullet, consistent with the existing bullets' length.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./agent/ -run TestPlannerPromptDocumentsBurnSignal -v`
Expected: PASS

- [ ] **Step 5: Run the whole `agent` package suite once more to confirm the prompt change broke nothing else**

Run: `go test ./agent/... -v`
Expected: PASS (no other test asserts on the exact unchanged text of `plannerDefaultTmpl` in a way a pure addition would break; if one does, that's a plan-wrong finding — ledger a ruling on the smallest fix)

- [ ] **Step 6: Commit**

```bash
git add agent/planner.go agent/planner_prompt_test.go
git commit -m "feat(agent): document the burn signal in the planner's system prompt"
```
