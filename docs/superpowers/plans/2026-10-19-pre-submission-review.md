# Pre-Submission Finding Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the user an on-demand, per-task "review before you submit" capability that produces a Markdown report on duplicate/same-root-cause findings, evidence gaps, and severity-downgrade risk — reachable via the HTTP API, with a persisted history of past runs.

**Architecture:** A new standalone conversation-backed agent (`triager`), wired into the existing generic `conversations`/`conversation_activities` machinery exactly the way the existing `retester` agent (`db/finding_retests.go` + `server/finding_retests.go`) is — same immutable-record-per-run table shape, same row-locked dedup-on-create, same pending→running→completed/failed/stopped lifecycle hook inside `runConversationTurn`, same trigger-less seeded-agent pattern. It reuses 7 pre-existing read tools already bound to other agents (no new read tools) and adds exactly one new write tool, `record_submission_review`.

**Tech Stack:** Go, PostgreSQL (`database/sql` + pgx), the existing `agent`/`server`/`db` packages, the `norma` SDK's `actool`/`llm` packages for tool/provider shapes.

**Spec:** `docs/superpowers/specs/2026-10-19-pre-submission-review-design.md`.

## Global Constraints

- **On-demand only, no automatic/periodic trigger** — confirmed in the design's Non-goals; the new agent `triager` is seeded the same trigger-less way `retester` is (no row in `agent_triggers`).
- **Advisory, never mutates findings** — `record_submission_review`/`FinishTaskSubmissionReview` never change a finding's severity, status, or asset bindings, and never merge findings in the database. (Non-goal, explicit anti-goal in the spec.)
- **No new read tools** — `triager` is bound to exactly these 7 pre-existing tools, unchanged: `list_task_findings`, `get_task_node_detail`, `get_task_worker_trace`, `list_task_worker_traces`, `search_task_worker_traces`, `get_task_graph`, `get_finding_traffic`.
- **No structured per-finding DB tagging** — the review's output is one free-text Markdown `summary` field, not a new schema of per-finding flags. (Explicit non-goal.)
- **No new UI** — reachable via the HTTP API only, mirroring exactly how `retester` is reachable (`/api/exploration/findings/{id}/retests` and friends).
- **`summary` cap: 128KB** (not 16KB — the spec's corrected value, matching `finding_retests`' `evidence` field cap, not its `summary` cap, because this single field carries everything a multi-finding review produces).
- **`record_submission_review` is callable more than once per running session** (checkpoint/overwrite semantics while `status='running'`, locked only once the session reaches a terminal status) — the spec's corrected value, deliberately looser than `finding_retests`' write-once-or-refuse verdict guard.
- **Seeding the 7 pre-existing tools uses `AddAgentToToolBinding`, never `SeedTool`** — `db.SeedTool` is `ON CONFLICT (key) DO NOTHING`, a no-op on a key that already has a row (all 7 already exist, seeded earlier for `reporter`/`auto`). `SeedTool` is reserved for the one genuinely new tool, `record_submission_review`.
- **Optional per-run `notes`** (≤4000 chars, mirroring `startFindingRetest`'s exact validation), appended to the initial message the same way `FindingRetest.InitialMessage()` appends its own.

## Review Focus

- **A tool call from a conversation with no linked review row** (e.g. an unrelated chat conversation somehow invoking `record_submission_review`) must be rejected, not silently accepted or misattributed — mirrors the existing `retester` tools' behavior, tested in `TestRetestValidationAndRollback`'s loop over `s.findingRetestTools()`.
- **A review session that completes without ever calling `record_submission_review`** must land as `status='failed'` with a clear error, not `'completed'` with an empty summary — mirrors `FinishFindingRetest`'s `CASE WHEN $2='completed' AND verdict='' THEN 'failed'` logic exactly, just keyed on `summary=''` instead of `verdict=''`.
- **Two concurrent `POST .../review` calls for the same task** must produce exactly one running review, not two — mirrors `TestRetestAtomicDeduplicationAndHistory`'s 8-goroutine dedup test, scoped by `task_id` instead of `finding_id`.
- **An oversized `summary` (>128KB)** must be hard-rejected with a clear error, not silently truncated or accepted.
- **A review targeting a task ID that doesn't exist** must 404, not 500 or silently create an orphaned row.

---

### Task 1: DB layer — `task_submission_reviews` table + CRUD

**Files:**
- Modify: `db/schema.sql` (append near the existing `finding_retests` table, §1137-1166)
- Create: `db/task_submission_reviews.go`
- Test: `db/task_submission_reviews_test.go`

**Interfaces:**
- Consumes: `d.BeginTx`/`scanConv`/`convCols` (existing `conversations` helpers, already used by `CreateFindingRetest`); `testDSN(t)` (existing test helper, used by `db/finding_retests_test.go`).
- Produces (consumed by Task 2): `db.TaskSubmissionReviewAgentKey` (const, value `"triager"`); `db.ErrReviewNotRunning` (error); `type db.TaskSubmissionReview struct { ID, TaskID int64; ConversationID *int64; Status, Notes, Summary, Error string; CreatedAt time.Time; StartedAt, FinishedAt *time.Time }`; `type db.ActiveTaskSubmissionReview struct { ID int64; TaskID int64 \`json:"task_id,string"\`; ConversationID int64; Status string }`; `func (d *DB) CreateTaskSubmissionReview(ctx context.Context, taskID int64, notes string) (*TaskSubmissionReview, *Conversation, bool, error)`; `func (r *TaskSubmissionReview) InitialMessage() string`; `func (d *DB) ListTaskSubmissionReviews(taskID int64) ([]*TaskSubmissionReview, error)`; `func (d *DB) ListActiveTaskSubmissionReviews(ctx context.Context) ([]ActiveTaskSubmissionReview, error)`; `func (d *DB) TaskSubmissionReviewForConversation(ctx context.Context, conversationID int64) (*TaskSubmissionReview, error)`; `func (d *DB) FailPendingTaskSubmissionReviewForConversation(conversationID int64, reason string) error`; `func (d *DB) StartTaskSubmissionReview(ctx context.Context, id int64) (bool, error)`; `func (d *DB) RecordTaskSubmissionReviewResult(ctx context.Context, conversationID int64, summary string) error`; `func (d *DB) FinishTaskSubmissionReview(id int64, status, reason string) error`; `func (d *DB) RecoverTaskSubmissionReviews() error`.

- [ ] **Step 1: Write the failing tests**

Create `db/task_submission_reviews_test.go`, following `db/finding_retests_test.go`'s structure exactly (read that file first — it is the template). Three tests:

```go
package db

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func reviewDB(t *testing.T) (*DB, int64) {
	t.Helper()
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	tk, err := d.CreateTask("review-test", "test the review feature", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM conversations WHERE id IN (SELECT conversation_id FROM task_submission_reviews WHERE task_id=$1)`, tk.ID)
		_ = d.DeleteTask(tk.ID) // cascades task_submission_reviews via ON DELETE CASCADE
		d.Close()
	})
	return d, tk.ID
}

func TestReviewAtomicDeduplicationAndHistory(t *testing.T) {
	d, taskID := reviewDB(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	createdCount := 0
	ids := make([]int64, 0, 8)
	for range 8 {
		wg.Go(func() {
			r, c, created, err := d.CreateTaskSubmissionReview(t.Context(), taskID, "focus on SSRF")
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			ids = append(ids, r.ID)
			if created {
				createdCount++
				if c == nil {
					t.Error("created without conversation")
				}
			}
		})
	}
	wg.Wait()
	if createdCount != 1 || len(ids) != 8 {
		t.Fatalf("created=%d ids=%v", createdCount, ids)
	}
	for _, id := range ids {
		if id != ids[0] {
			t.Fatal("duplicate active reviews", ids)
		}
	}
	rows, err := d.ListTaskSubmissionReviews(taskID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("history=%v err=%v", rows, err)
	}
	r := rows[0]
	ctx := t.Context()
	if err := d.RecordTaskSubmissionReviewResult(ctx, *r.ConversationID, "draft"); !errors.Is(err, ErrReviewNotRunning) {
		t.Fatal("pending accepted result", err)
	}
	if ok, err := d.StartTaskSubmissionReview(ctx, r.ID); err != nil || !ok {
		t.Fatalf("start=%t %v", ok, err)
	}
	if err := d.RecordTaskSubmissionReviewResult(ctx, 0, "x"); !errors.Is(err, ErrReviewNotRunning) {
		t.Fatal("unscoped write accepted", err)
	}
	// Checkpoint semantics: two different calls while running both succeed, second wins.
	if err := d.RecordTaskSubmissionReviewResult(ctx, *r.ConversationID, "first pass: 3 findings reviewed"); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordTaskSubmissionReviewResult(ctx, *r.ConversationID, "final: 26 findings, 10 duplicate groups"); err != nil {
		t.Fatal(err)
	}
	if err := d.FinishTaskSubmissionReview(r.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordTaskSubmissionReviewResult(ctx, *r.ConversationID, "too late"); !errors.Is(err, ErrReviewNotRunning) {
		t.Fatal("overwrote sealed result", err)
	}
	rows, _ = d.ListTaskSubmissionReviews(taskID)
	if len(rows) != 1 || rows[0].Status != "completed" || rows[0].Summary != "final: 26 findings, 10 duplicate groups" {
		t.Fatal("checkpoint overwrite did not win", rows)
	}

	// A second review after the first finishes is new history, and a review that
	// finishes with no summary at all must be marked failed.
	next, _, created, err := d.CreateTaskSubmissionReview(ctx, taskID, "")
	if err != nil || !created || next.ID == r.ID {
		t.Fatalf("new history=%+v created=%t err=%v", next, created, err)
	}
	if err := d.FinishTaskSubmissionReview(next.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	rows, _ = d.ListTaskSubmissionReviews(taskID)
	if len(rows) != 2 || rows[0].Status != "failed" || rows[0].Error == "" {
		t.Fatal("missing summary treated as successful", rows)
	}
}

func TestReviewDeletionAndRestart(t *testing.T) {
	d, taskID := reviewDB(t)
	r, c, _, err := d.CreateTaskSubmissionReview(t.Context(), taskID, "delete")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteConversation(c.ID); err != nil {
		t.Fatal(err)
	}
	rows, _ := d.ListTaskSubmissionReviews(taskID)
	if len(rows) != 1 || rows[0].ConversationID != nil || rows[0].Status != "stopped" {
		t.Fatal(rows)
	}
	next, c2, created, err := d.CreateTaskSubmissionReview(t.Context(), taskID, "restart")
	if err != nil || !created {
		t.Fatal("deleted session blocks retry", err)
	}
	if ok, err := d.StartTaskSubmissionReview(t.Context(), next.ID); err != nil || !ok {
		t.Fatal(err)
	}
	if err := d.RecoverTaskSubmissionReviews(); err != nil {
		t.Fatal(err)
	}
	if err := d.FinishTaskSubmissionReview(next.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	full, _ := d.TaskSubmissionReviewForConversation(t.Context(), c2.ID)
	if full.Status != "stopped" || full.FinishedAt == nil {
		t.Fatal("restart result overwritten", full)
	}
	_ = d.DeleteConversation(c2.ID)
	_, _ = d.Exec(`DELETE FROM task_submission_reviews WHERE id IN ($1,$2)`, r.ID, next.ID)
}

func TestReviewValidation(t *testing.T) {
	d, taskID := reviewDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, _, err := d.CreateTaskSubmissionReview(ctx, taskID, ""); err == nil {
		t.Fatal("cancelled creation succeeded")
	}
	for _, s := range []string{"", strings.Repeat("x", 128001)} {
		if err := d.RecordTaskSubmissionReviewResult(t.Context(), 0, s); err == nil {
			t.Fatal("bad result accepted, len", len(s))
		}
	}
	if _, _, _, err := d.CreateTaskSubmissionReview(t.Context(), 999999999, ""); err == nil {
		t.Fatal("review created for a nonexistent task")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./db/ -run TestReview -v`
Expected: FAIL (`CreateTaskSubmissionReview` and friends undefined). Before implementing, find the existing task-creation test helper this package already uses (grep `db/*_test.go` for how a task row gets created in a db-package test, e.g. in `db/tasks_test.go`) and replace the placeholder `taskID := int64(0)` in `reviewDB` with a real call to it — do not invent a new one.

- [ ] **Step 3: Implement `db/task_submission_reviews.go`**

Mirror `db/finding_retests.go` line for line where the shape matches, with these specific deltas (all already decided in the spec, not open choices):
- `TaskSubmissionReviewAgentKey = "triager"`.
- No `Verdict` field, no `Snapshot` field (the spec's non-goals: free-form report, no point-in-time snapshot since `list_task_findings` always reads live).
- `CreateTaskSubmissionReview` locks the `tasks` row instead of a `findings` row: `SELECT 1 FROM tasks WHERE id=$1 FOR UPDATE` (plain existence+lock check; `sql.ErrNoRows` on a missing task propagates up for Task 2's handler to turn into a 404, exactly as `startFindingRetest` does today for a missing finding).
- Conversation title: `fmt.Sprintf("submission review - task #%d", taskID)` (no need to read task name/description — unlike a finding, the task is already identified by the URL path the caller used).
- `InitialMessage()`: `fmt.Sprintf("Please review task #%d's current findings before submission. Call list_task_findings(task_id=%d) first for the severity/evidence overview (each entry's traffic_count tells you whether real evidence is bound), then spot-check specific findings with get_finding_traffic/get_task_node_detail as needed. Finally call record_submission_review with a Markdown report covering: (1) duplicate or same-root-cause findings that should be merged before submission, (2) findings with missing or weak evidence, (3) findings at risk of being downgraded in triage because their impact is inferred rather than observed.", r.TaskID, r.TaskID)`, then append `"\n\nAdditional notes for this review:\n" + r.Notes` when `r.Notes != ""` (identical structure to `FindingRetest.InitialMessage()`).
- `RecordTaskSubmissionReviewResult(ctx, conversationID, summary)`: trims `summary`, rejects empty, rejects `len(summary) > 128000` ("the review is too long (summary <= 128KB)"), then `UPDATE task_submission_reviews SET summary=$2 WHERE conversation_id=$1 AND status='running'` — no identical-or-refuse guard (checkpoint semantics: this always overwrites while running). Zero rows affected → `ErrReviewNotRunning`.
- `FinishTaskSubmissionReview(id, status, reason)`: no transaction and no companion-table update needed (unlike `FinishFindingRetest`, there is no triage side effect) — a single `UPDATE task_submission_reviews SET status=CASE WHEN $2='completed' AND summary='' THEN 'failed' ELSE $2 END, error=CASE WHEN $2='completed' AND summary='' THEN 'the agent saved no review; check the session and try again' ELSE $3 END, finished_at=now() WHERE id=$1 AND status IN ('pending','running')`.
- `RecoverTaskSubmissionReviews()`: `UPDATE task_submission_reviews SET status='stopped', error='the service restarted and the review was interrupted; please start it again', finished_at=now() WHERE status IN ('pending','running')`.

Add to `db/schema.sql` (immediately after the `finding_retests` block, §1137-1166):

```sql
CREATE TABLE IF NOT EXISTS task_submission_reviews (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    conversation_id BIGINT UNIQUE REFERENCES conversations(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed','stopped')),
    notes TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_task_submission_reviews_history ON task_submission_reviews(task_id, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_submission_reviews_active ON task_submission_reviews(task_id)
    WHERE status IN ('pending','running');

-- Deleting a session keeps the review records and releases any review still in progress.
CREATE OR REPLACE FUNCTION stop_deleted_conversation_task_review() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE task_submission_reviews SET status='stopped', error='the review session was deleted', finished_at=now()
    WHERE conversation_id=OLD.id AND status IN ('pending','running');
    RETURN OLD;
END;
$$;
DROP TRIGGER IF EXISTS trg_conversation_task_review_delete ON conversations;
CREATE TRIGGER trg_conversation_task_review_delete BEFORE DELETE ON conversations
    FOR EACH ROW EXECUTE FUNCTION stop_deleted_conversation_task_review();
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./db/ -run TestReview -v`
Expected: PASS (3/3)

- [ ] **Step 5: Commit**

```bash
git add db/schema.sql db/task_submission_reviews.go db/task_submission_reviews_test.go
git commit -m "feat(db): add the task_submission_reviews table and CRUD"
```

---

### Task 2: Agent prompt + tool + API + conversation-runner wiring

**Files:**
- Create: `agent/triager.go`
- Create: `server/task_submission_reviews.go`
- Modify: `server/conversations.go` (the `runConversationTurn` special-case block, ~lines 504-538)
- Modify: `server/server.go` (seed call near line 225; routes near lines 768-770)
- Modify: `server/manager.go` (recovery call near line 375)
- Test: `server/task_submission_reviews_test.go`

**Interfaces:**
- Consumes: everything Task 1 produces (`db.TaskSubmissionReviewAgentKey`, `db.TaskSubmissionReview`, `db.ActiveTaskSubmissionReview`, and all the `db.*TaskSubmissionReview*` functions).
- Consumes (pre-existing, unchanged): `roTool`/`wrTool`/`objSchema`/`strParam`/`jsonResult` (`server/orchestration.go`); `decodeConversationRequest`, `s.convBusyKey`, `s.resolveChatAgent`, `s.chatUnavailableReason`, `s.runConversation` (`server/conversations.go`); `s.pg(w)`, `pathInt` (`server/server_mgmt.go`); `s.m.Task(id string) (*Task, bool)` (`server/manager.go`); `intercept.ConvIDFromContext(ctx)` (`intercept` package, already used by `findingRetestTools`).
- Produces: nothing consumed by a later task — this is the plan's final task.

- [ ] **Step 1: Write the failing tests**

Create `server/task_submission_reviews_test.go`, following `server/finding_retests_test.go`'s structure exactly (read that file first — it is the template: `retestProvider`, `newRetestServer`, `setRetestProvider`, `retestRequest`, `waitRetestIdle` all have a direct analog below). Four tests mirroring the four in the template:

```go
package server

// Mirror server/finding_retests_test.go's retestProvider/newRetestServer/
// setRetestProvider/retestRequest/waitRetestIdle helpers, renamed for reviews:
// reviewProvider, newReviewServer, setReviewProvider, reviewRequest, waitReviewIdle.
// newReviewServer additionally needs a real task row (not just a finding row) --
// find the existing task-creation test helper this package already uses for
// server-level tests (grep server/*_test.go for how a *Task gets created, e.g.
// via m.CreateTask, the same way server/harness_fixture_test.go's setupHarness
// does it) and use that instead of inventing a new one. newReviewServer calls
// s.seedTaskSubmissionReviewer() (this task's Step 3) instead of s.seedFindingRetester().

// TestReviewActiveStatusLifecycle: mirrors TestRetestActiveStatusLifecycle --
// pending -> running -> each terminal status (completed/failed/stopped) shows up
// correctly in listActiveTaskSubmissionReviews and then disappears, and the
// active-status response never leaks "summary" or "notes".

// TestReviewHTTPThroughConversationAndTools: mirrors TestRetestHTTPThroughConversationAndTools --
// a scripted provider calls list_task_findings-equivalent (any bound read tool)
// then record_submission_review, startTaskSubmissionReview returns 202, a duplicate
// concurrent POST returns 200 "created":false, and after the turn ends
// listTaskSubmissionReviews shows one completed row with the saved summary.

// TestReviewStopsFailuresAndNoSummary: mirrors TestRetestStopsFailuresAndNoVerdict --
// stop/failure/no-conclusion all land on the right terminal status with a non-empty error.

// TestReviewValidationSeedingAndScope: mirrors TestRetestValidationAndRollback's second half --
// bad task id / malformed body / notes over 4000 chars / nonexistent task / service
// unavailable all produce the right HTTP status with no state created; every tool in
// s.taskSubmissionReviewTools() rejects a call from an unassociated conversation;
// seedTaskSubmissionReviewer() is idempotent and never overwrites a user-edited prompt;
// and (Review Focus: no automatic trigger) agent_triggers has zero rows for
// db.TaskSubmissionReviewAgentKey after seeding.
```

Write these four tests in full, in the style of the four tests in `server/finding_retests_test.go` — same assertions, same structure, substituting the task-scoped names above. The implementer has the template file open; do not restate its code here.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./server/ -run TestReview -v`
Expected: FAIL (compile error: `seedTaskSubmissionReviewer`, `startTaskSubmissionReview`, etc. undefined)

- [ ] **Step 3: Implement `agent/triager.go`**

```go
package agent

// TriagerDefaultPrompt is seeded once as an editable conversation agent.
const TriagerDefaultPrompt = `...`
```

Write the prompt following `agent/retester.go`'s exact style and numbered-instruction structure, covering: (1) call `list_task_findings` first for the overview, (2) spot-check with `get_finding_traffic`/`get_task_node_detail`/the trace tools as needed, (3) never alter findings, never create/merge/delete anything, this is a read-only analytical pass, (4) the three required report dimensions (duplicate/merge candidates, evidence gaps, severity-downgrade risk), (5) call `record_submission_review(summary)` to save it — may be called more than once as a checkpoint, but only the last call before the session ends is kept, (6) stay concise, do not re-explore the target.

- [ ] **Step 4: Implement `server/task_submission_reviews.go`**

Mirror `server/finding_retests.go` in full (handlers + tools + seed function), with these deltas:

```go
func (s *Server) listActiveTaskSubmissionReviews(w http.ResponseWriter, r *http.Request)
func (s *Server) listTaskSubmissionReviews(w http.ResponseWriter, r *http.Request)   // path param "id" = task id (string, via s.m.Task)
func (s *Server) startTaskSubmissionReview(w http.ResponseWriter, r *http.Request)
func (s *Server) taskSubmissionReviewTools() []actool.CoreTool
func (s *Server) seedTaskSubmissionReviewer() error
func (s *Server) finishTaskSubmissionReview(id int64, status, reason string)
```

- `listTaskSubmissionReviews`/`startTaskSubmissionReview` resolve the task with `s.m.Task(r.PathValue("id"))` (NOT `pathInt` — `Task.ID` is a `string`, per `server/manager.go`'s `Task` struct), returning 404 when `!ok`; then `strconv.ParseInt(t.ID, 10, 64)` for the `int64` the `db` layer needs.
- `startTaskSubmissionReview` otherwise mirrors `startFindingRetest` exactly: decode `{"notes": "..."}` via `decodeConversationRequest`, trim+cap `notes` at 4000 runes (same error message shape), check `GetAgentByKey(db.TaskSubmissionReviewAgentKey)` is non-nil and enabled, check `record_submission_review` is enabled and bound to it (the 7 pre-existing tools are assumed already enabled — they are shared with `reporter`/`auto`, not re-checked here), check `s.resolveChatAgent`/`s.chatUnavailableReason`/`s.ctx.Err()`, call `pg.CreateTaskSubmissionReview`, and on `created` call `s.runConversation(conv, review.InitialMessage(), s.convBusyKey(conv.ID))` under the same `chatBusy` bookkeeping `startFindingRetest` uses. Returns `202` + `{"review": review, "created": true}` on creation, `200` + `{"review": review, "created": false}` on dedup.
- `taskSubmissionReviewTools()` returns exactly one tool, `record_submission_review`:
  ```go
  wrTool("record_submission_review", "Save this review session's report. Overwrites any earlier checkpoint from this same session; only the last call before the session ends is kept. The original findings are never modified.",
      objSchema(map[string]any{"summary": strParam("the review report, Markdown: duplicate/merge candidates, evidence gaps, severity-downgrade risk")}, "summary"),
      func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
          var a struct{ Summary string `json:"summary"` }
          if err := json.Unmarshal(in, &a); err != nil { return actool.Errorf(err.Error()), nil }
          if err := s.m.pg.RecordTaskSubmissionReviewResult(ctx, intercept.ConvIDFromContext(ctx), a.Summary); err != nil {
              return actool.Errorf(err.Error()), nil
          }
          return jsonResult(map[string]any{"saved": true})
      })
  ```
- `seedTaskSubmissionReviewer()` mirrors `seedFindingRetester()`'s transaction/advisory-lock/settings-flag shape exactly, with these deltas: (a) `SeedTool` is called only for `record_submission_review`, binding `[]string{db.TaskSubmissionReviewAgentKey}`; (b) after the advisory-lock-guarded agent/prompt creation block (flag e.g. `"task_submission_review_seed_v1"`, a **new, unique** advisory lock key distinct from `7337741010` — e.g. `7337741020`), call `s.m.pg.AddAgentToToolBinding(db.TaskSubmissionReviewAgentKey, []string{"list_task_findings", "get_task_node_detail", "get_task_worker_trace", "list_task_worker_traces", "search_task_worker_traces", "get_task_graph", "get_finding_traffic"})` to bind the 7 pre-existing tools (additive, idempotent — safe to call on every startup, unlike `SeedTool`). Use `agent.TriagerDefaultPrompt` for the seeded prompt and `"assistant"` for the agent's `role`, matching `retester`.
- `finishTaskSubmissionReview` mirrors `finishRetest` exactly: `if err := s.m.pg.FinishTaskSubmissionReview(id, status, reason); err != nil { log.Printf("[triager] finish: %v", err) }`.

- [ ] **Step 5: Wire the conversation-runner lifecycle in `server/conversations.go`**

In `runConversationTurn` (~lines 504-538), the existing block is:

```go
finishStatus, finishReason := "failed", "the retest could not be started"
if c.AgentKey == db.FindingRetestAgentKey {
    // ... load retest, defer finishRetest, StartFindingRetest ...
}
```

Add a parallel `else if c.AgentKey == db.TaskSubmissionReviewAgentKey` branch with the identical structure, substituting: `s.m.pg.TaskSubmissionReviewForConversation` for `FindingRetestForConversation`, `s.m.pg.FailPendingTaskSubmissionReviewForConversation` for `FailPendingRetestForConversation`, `s.finishTaskSubmissionReview` for `s.finishRetest`, `s.m.pg.StartTaskSubmissionReview` for `StartFindingRetest`. Update the shared `finishReason` default string ("the retest could not be started") to something agent-agnostic (e.g. "the session could not be started") since it is now shared by two branches, or set it per-branch right before entering each — either is fine, pick whichever reads more clearly once both branches exist side by side.

- [ ] **Step 6: Wire startup seeding and recovery**

In `server/server.go`, immediately after the existing `if err := s.seedFindingRetester(); err != nil { log.Printf("[retester] seed: %v", err) }` block (~line 225):

```go
if err := s.seedTaskSubmissionReviewer(); err != nil {
    log.Printf("[triager] seed: %v", err)
}
```

In `server/manager.go`'s `NewManager`, immediately after the existing `if err := pg.RecoverFindingRetests(); err != nil { ... }` block (~line 375):

```go
if err := pg.RecoverTaskSubmissionReviews(); err != nil {
    pg.Close()
    return nil, fmt.Errorf("recover task submission reviews: %w", err)
}
```

In `server/server.go`'s route table, immediately after the existing retest routes (~lines 768-770):

```go
mux.HandleFunc("GET /api/tasks/{id}/findings/reviews", s.listTaskSubmissionReviews)
mux.HandleFunc("GET /api/tasks/findings/reviews/active", s.listActiveTaskSubmissionReviews)
mux.HandleFunc("POST /api/tasks/{id}/findings/review", s.startTaskSubmissionReview)
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./server/ -run TestReview -v`
Expected: PASS (4/4)

- [ ] **Step 8: Run the whole `agent`, `db`, and `server` package suites to confirm nothing else regressed**

Run: `go test ./agent/... ./db/... ./server/... -p 1 -v`
Expected: PASS (or only the already-documented pre-existing baseline flakiness from this session's prior workstreams — no new failures)

- [ ] **Step 9: Commit**

```bash
git add agent/triager.go server/task_submission_reviews.go server/task_submission_reviews_test.go server/conversations.go server/server.go server/manager.go
git commit -m "feat(server,agent): add the on-demand pre-submission finding review"
```
