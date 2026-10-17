# W0: Planner/Worker Integration Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a minimal integration-test harness in the `server` package that drives one real planner round → intent dispatch → worker execution → tool call → graph write-back, against a fake (programmatic) LLM provider and a fake HTTP target, backed by a real test Postgres — so later workstreams (burn-signal visibility, reachability edges, the stateful HTTP tool) have an automated regression gate that does not exist today.

**Architecture:** A scriptable fake `llm.Provider` returns a pre-programmed sequence of `llm.Message`s (one per planner/worker turn) instead of calling a real model. A real `*server.Engine` is constructed against a real (throwaway) Postgres database via the existing `NewManager`/`CreateTask` path, with `Engine.SetAuthoritativeAgentResolver` swapped to return `agent.Planner`/`agent.Worker` instances built on the fake provider instead of the normal DB-resolved one. The engine is run for real (`Engine.Run`) against a cancellable context; the test polls the real `ExplorationStore` for the expected graph state (an intent, then a fact) rather than sleeping a fixed duration.

**Tech Stack:** Go, `testing`, `net/http/httptest`, Postgres (via `ARTEX_PG_DSN`), the vendored `github.com/Autumn-27/norma` SDK (`llm.Provider`, `agentcore`), this repo's `server`/`agent`/`db` packages.

**Spec:** `docs/superpowers/specs/2026-10-17-core-engine-finding-capability-design.md` (workstream W0).

## Global Constraints

- Tests require `ARTEX_PG_DSN` (or an equivalent `config.json` database section) pointing at a reachable, throwaway Postgres database; schema application is automatic (`db.Open` applies `schema.sql` idempotently — no manual migration step).
- Tests must be **skip-tolerant**, not fail-the-suite, when Postgres is unavailable — matches the existing convention (`server/core_test.go:20-25`'s `t.Skipf("postgres unavailable (%v) — skipping", err)`), so this harness does not break `go test ./...` runs that have no DB configured (the same environment this plan was researched in had none).
- No schema changes in this workstream — purely new test-only Go files in `server/`.
- Every test must clean up what it creates in Postgres (`t.Cleanup`), matching `server/finding_traffic_test.go:23-66`'s pattern, so repeated local runs don't accumulate garbage rows in a shared dev database.

## Review Focus

- **Postgres unavailable** (no `ARTEX_PG_DSN` set, e.g. plain CI or a fresh dev machine) → the harness must skip cleanly via `t.Skipf`, not fail or hang. (Task 2)
- **Engine never produces the scripted intent/fact** (scripted tool-call names don't match what the real `add_intent`/`WebFetch`/`record_fact` tool schemas expect) → the test must fail with a clear, diagnosable message (last observed graph state), not a bare "timed out after 30s" with no context. (Task 4)
- **Goroutine/context leak** (`Engine.Run`'s `plannerLoop`/`workerLoop` goroutines are infinite `select` loops that only stop on context cancellation) → the test must cancel its context before returning, every time, including on an assertion failure partway through polling (use `defer cancel()`, not a cancel call only on the success path). (Task 4)
- **Concurrent calls into the fake provider** (the real `plannerLoop` and `workerLoop` goroutines run concurrently once `Engine.Run` starts, and both call into the same injected `llm.Provider`) → the scripted provider's internal response-index counter must be safe for concurrent `Complete`/`Stream` calls, or two goroutines racing to pop the next canned response will silently scramble the script order. (Task 1)
- **Test pollutes a shared/dev Postgres instance** across repeated local runs → every task/exploration row the harness creates must be deleted via `t.Cleanup`, matching the existing `trafficEvidenceServer` pattern, not left behind. (Task 2)

---

### Task 1: Scriptable fake LLM provider

**Files:**
- Create: `server/harness_fakellm_test.go`
- Test: same file (test-only Go file, package `server`)

**Interfaces:**
- Consumes: `llm.Provider` interface (`Stream(ctx, req) iter.Seq2[llm.StreamEvent, error]`, `Complete(ctx, req) (llm.Message, string, llm.Usage, error)`) — `~/go/pkg/mod/github.com/!autumn-27/norma@v0.4.3/llm/provider.go:160-163`.
- Produces: `newScriptedProvider(responses ...llm.Message) *scriptedProvider`, a type implementing `llm.Provider` that returns the given `llm.Message`s in order, one per `Complete`/`Stream` call, and returns a sentinel error (`errScriptExhausted`) once the sequence is consumed. Later tasks (3, 4) construct this with the planner/worker turns they need and pass it to `agent.NewPlanner`/`agent.NewWorker`.

- [ ] **Step 1: Write the failing test**

```go
func TestScriptedProviderReturnsResponsesInOrder(t *testing.T) {
    m1 := llm.Message{ /* minimal valid assistant message, see note below */ }
    m2 := llm.Message{ /* a second, distinguishable message */ }
    p := newScriptedProvider(m1, m2)

    got1, _, _, err := p.Complete(context.Background(), llm.CompletionRequest{})
    if err != nil || !reflect.DeepEqual(got1, m1) {
        t.Fatalf("call 1: got %+v, err %v; want %+v, nil", got1, err, m1)
    }
    got2, _, _, err := p.Complete(context.Background(), llm.CompletionRequest{})
    if err != nil || !reflect.DeepEqual(got2, m2) {
        t.Fatalf("call 2: got %+v, err %v; want %+v, nil", got2, err, m2)
    }
    if _, _, _, err := p.Complete(context.Background(), llm.CompletionRequest{}); !errors.Is(err, errScriptExhausted) {
        t.Fatalf("call 3: got err %v, want errScriptExhausted", err)
    }
}
```

Before writing `m1`/`m2`, read `~/go/pkg/mod/github.com/!autumn-27/norma@v0.4.3/llm/provider.go` (and the file defining `llm.Message`/`llm.ContentBlock`/`llm.Usage` alongside it) for the exact struct fields, and build `m1`/`m2` as plain text-content assistant messages distinguishable by content (this step has no remaining design decision — it's a mechanical type lookup).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestScriptedProviderReturnsResponsesInOrder -v`
Expected: FAIL (`scriptedProvider`/`newScriptedProvider`/`errScriptExhausted` undefined)

- [ ] **Step 3: Implement `scriptedProvider`**

A struct holding `responses []llm.Message`, an index, and a `sync.Mutex` (per Review Focus: concurrent-safety, since planner and worker goroutines will call this concurrently in Task 4). `Complete` locks, reads-and-increments the index, returns `(responses[i], "", llm.Usage{}, nil)` or `(llm.Message{}, "", llm.Usage{}, errScriptExhausted)` once exhausted. `Stream` wraps the same logic, yielding the message as a single `llm.StreamEvent` (read `agent/provider_capture_e2e_test.go`'s consumption of `prov.Stream(...)` to confirm which `StreamEvent` variant/field represents a complete assistant message) then ending the sequence — no network/SSE involved, this is a pure in-process fake.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/ -run TestScriptedProviderReturnsResponsesInOrder -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/harness_fakellm_test.go
git commit -m "test(server): add scriptable fake LLM provider for integration harness"
```

---

### Task 2: Postgres-backed fixture + fake HTTP target

**Files:**
- Create: `server/harness_fixture_test.go`
- Test: same file

**Interfaces:**
- Consumes: `NewManager(dir, proxyAddr string) (*Manager, error)` (`server/manager.go:344`), `(*Manager).CreateTask(description, goal string, llmProfileID *int64, timeoutSeconds, planHeartbeatSeconds int) (*Task, error)` (`server/manager.go:881`).
- Produces: `setupHarness(t *testing.T) (m *Manager, task *Task, target *httptest.Server)` — skip-tolerant (calls `t.Skipf` on a Postgres-unavailable error, never `t.Fatal`), creates one task via `CreateTask`, stands up an `httptest.NewServer` that replies `200 OK` with a fixed, distinguishable body (e.g. `"fake-target-ok"`) to any request, and registers `t.Cleanup` to close the fake server and delete the created task/manager. Used by Tasks 3 and 4.

- [ ] **Step 1: Write the failing test**

```go
func TestSetupHarnessCreatesTaskAndTarget(t *testing.T) {
    m, task, target := setupHarness(t)
    if m == nil || task == nil || task.Store == nil {
        t.Fatalf("setupHarness returned incomplete fixture: m=%v task=%v store=%v", m, task, task.Store)
    }
    resp, err := http.Get(target.URL)
    if err != nil {
        t.Fatalf("fake target unreachable: %v", err)
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    if string(body) != "fake-target-ok" {
        t.Fatalf("fake target body = %q, want %q", body, "fake-target-ok")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestSetupHarnessCreatesTaskAndTarget -v`
Expected: FAIL (`setupHarness` undefined)

- [ ] **Step 3: Implement `setupHarness`**

Follow `server/core_test.go:20-25`'s skip pattern (`m, err := NewManager(t.TempDir(), ""); if err != nil { t.Skipf(...) }`) and `server/finding_traffic_test.go:23-66`'s task-creation pattern (`m.CreateTask("harness e2e", "harness goal", nil, 0, 0)`). Stand up the fake target with `httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "fake-target-ok") }))`. Register `t.Cleanup(func() { target.Close(); /* delete task */; m.Close() })`.

- [ ] **Step 4: Run test to verify it passes**

Run (with `ARTEX_PG_DSN` set to a reachable throwaway Postgres): `go test ./server/ -run TestSetupHarnessCreatesTaskAndTarget -v`
Expected: PASS. Also run once with `ARTEX_PG_DSN` unset to confirm it reports `SKIP`, not `FAIL`.

- [ ] **Step 5: Commit**

```bash
git add server/harness_fixture_test.go
git commit -m "test(server): add Postgres-backed harness fixture + fake HTTP target"
```

---

### Task 3: Engine wiring onto the fake provider

**Files:**
- Modify: `server/harness_fixture_test.go` (add alongside `setupHarness`)

**Interfaces:**
- Consumes: `NewEngine(m *Manager) *Engine` (`server/engine.go:390`), `(*Engine).SetAuthoritativeAgentResolver(fn func(t *Task) (*agent.Planner, *agent.Worker))` (`server/engine.go:720`), `agent.NewPlanner(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *Planner` (`agent/planner.go:51`), `agent.NewWorker(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int, extra ...actool.CoreTool) *Worker` (`agent/worker.go:156`), `scriptedProvider` (Task 1).
- Produces: `wireHarnessEngine(t *testing.T, m *Manager, prov llm.Provider) *Engine` — builds one `*agent.Planner` and one `*agent.Worker` on `prov` (generous `window=200000`, `maxTurns=10`, `tx=nil` — transcript persistence is optional per `agent/worker.go:62`/`agent/planner.go:27` and not needed here), installs them via `SetAuthoritativeAgentResolver` returning them unconditionally for any task, and returns the wired `*Engine`. Used by Task 4.

- [ ] **Step 1: Write the failing test, or document why none is written**

First check whether `Engine` exposes any way to read back the installed resolver or invoke it directly outside of `Run`. If it does, write `TestWireHarnessEngineResolverReturnsFakeAgents` asserting the resolver returns the exact `*agent.Planner`/`*agent.Worker` instances passed in. If it does not (no getter, and the resolver is only invoked internally by `plannerLoop`/`workerLoop`), skip a standalone test for this function — its correctness is verified end to end by Task 4 — and note that explicitly in this file's comments instead of inventing a getter the engine doesn't have.

- [ ] **Step 2: Run test to verify it fails (if Step 1 produced a test)**

Run: `go test ./server/ -run TestWireHarnessEngineResolverReturnsFakeAgents -v`
Expected: FAIL (`wireHarnessEngine` undefined)

- [ ] **Step 3: Implement `wireHarnessEngine`**

```go
func wireHarnessEngine(t *testing.T, m *Manager, prov llm.Provider) *Engine
```
Construct `pl := agent.NewPlanner(prov, "harness-fake-model", t.TempDir(), nil, 200000, 10)` and `wk := agent.NewWorker(prov, "harness-fake-model", t.TempDir(), nil, 200000, 10)`, `e := NewEngine(m)`, `e.SetAuthoritativeAgentResolver(func(*Task) (*agent.Planner, *agent.Worker) { return pl, wk })`, return `e`.

- [ ] **Step 4: Run test to verify it passes (if Step 1 produced a test), otherwise proceed to Task 4 for end-to-end verification**

Run: `go test ./server/ -run TestWireHarnessEngineResolverReturnsFakeAgents -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/harness_fixture_test.go
git commit -m "test(server): wire fake planner/worker into a real Engine for the harness"
```

---

### Task 4: End-to-end driver test

**Files:**
- Create: `server/harness_e2e_test.go`
- Test: same file

**Interfaces:**
- Consumes: `newScriptedProvider` (Task 1), `setupHarness` (Task 2), `wireHarnessEngine` (Task 3), `(*Engine).Run(ctx context.Context, t *Task)` (`server/engine.go:755`), the `*db.ExplorationStore` read method used by `agent/tools.go` to list nodes by kind (confirm its exact name/signature, e.g. `ListByKind(kind, limit)`, by grepping `agent/tools.go` before writing the assertion — this is a mechanical lookup, not a design decision).
- Produces: nothing consumed by later tasks — this is the workstream's deliverable.

- [ ] **Step 1: Write the end-to-end test**

```go
func TestEngineDrivesOnePlannerRoundThroughWorkerDispatch(t *testing.T) {
    m, task, target := setupHarness(t)

    // Turn 1 (planner): call add_intent with a fixed, recognizable summary.
    // Turn 2 (worker): call WebFetch against target.URL.
    // Turn 3 (worker): call record_fact with a fixed, recognizable body.
    // Turn 4 (worker): plain-text stop, ending the run.
    // Build each llm.Message's tool_use content block using the real tool
    // names/schemas from agent/tools.go (add_intent, record_fact) and the
    // SDK's WebFetch tool name — read agent/tools.go's addIntent()/recordFact()
    // schemas to get required fields right, or the worker will reject the call.
    prov := newScriptedProvider(plannerAddIntentMsg, workerWebFetchMsg(target.URL), workerRecordFactMsg, workerStopMsg)

    engine := wireHarnessEngine(t, m, prov)

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    go engine.Run(ctx, task)

    deadline := time.Now().Add(30 * time.Second)
    var lastState string
    for time.Now().Before(deadline) {
        facts, err := task.Store.ListByKind(db.KindFact, 10) // confirm exact name/signature before use
        if err == nil && len(facts) > 0 {
            for _, f := range facts {
                if strings.Contains(string(f.Payload), "harness-fact-marker") {
                    return // success
                }
            }
        }
        lastState = fmt.Sprintf("facts=%d err=%v", len(facts), err)
        time.Sleep(200 * time.Millisecond)
    }
    t.Fatalf("timed out waiting for scripted fact; last observed state: %s", lastState)
}
```

- [ ] **Step 2: Run test to verify it fails for the right reason**

Run: `go test ./server/ -run TestEngineDrivesOnePlannerRoundThroughWorkerDispatch -v`
Expected: FAIL — must be a real assertion/timeout failure (the harness ran but the scripted tool-call content didn't match what `add_intent`/`record_fact`'s real schemas expect), not a compile error. A compile error means something from Tasks 1-3 doesn't match this task's usage and must be fixed first.

- [ ] **Step 3: Fix the scripted message content until the loop actually produces the fact**

Iterate on the exact JSON shape of each scripted `tool_use` block against `agent/tools.go`'s `addIntent()`/`recordFact()` schemas (and the SDK's WebFetch tool schema) until the real tool handlers accept the calls and the fact gets written with the `"harness-fact-marker"` content.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/ -run TestEngineDrivesOnePlannerRoundThroughWorkerDispatch -v`
Expected: PASS

- [ ] **Step 5: Run the whole package to confirm nothing else regressed**

Run: `go test ./server/... -v`
Expected: PASS (or pre-existing, unrelated `ARTEX_PG_DSN`-missing skips only — no new failures)

- [ ] **Step 6: Commit**

```bash
git add server/harness_e2e_test.go
git commit -m "test(server): drive one planner round through worker dispatch end to end"
```
