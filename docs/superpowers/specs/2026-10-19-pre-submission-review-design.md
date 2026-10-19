# Pre-Submission Finding Review Design

## Intent and why

On the user's live task #15 (a Bugcrowd engagement against Linktree, 26 confirmed findings), asking the task's own main agent to manually review its findings surfaced, in one pass:

- **26/26 findings had `traffic_count = 0`** — zero real HTTP evidence bound to any finding, even on 3 spot-checked High findings.
- **~10 duplicate/same-root-cause groups** that should be merged before submission (e.g. two findings sharing the same missing-ownership-guard class; a subdomain-takeover finding missing ~250 same-root-cause hosts already sitting in the graph as facts).
- **~14 findings at risk of being downgraded to Low/informational** in triage, because their impact is inferred rather than directly observed.

This is valuable, concrete triage work — but today it only happens if a human thinks to ask the main agent to do it, in free-form chat, with no record kept afterward. The goal of this design is to make that same analysis a first-class, repeatable, on-demand capability: run it when you're about to submit, get a structured answer, and keep a history of past runs.

## Goals

- An on-demand (not automatic) review of one task's current findings that reports: duplicate/merge candidates, evidence-completeness gaps, and severity/downgrade risk — the same three dimensions already proven valuable above.
- Each run is persisted as an immutable record (mirroring how `finding_retests` already works), so past reviews stay visible even after findings change.
- Reachable the same way everything else in this system is reachable right now: a JSON HTTP API under the existing auth model. No new UI page is in scope for this design (see Non-goals).

## Non-goals

- **No automatic/periodic trigger.** Decided explicitly after comparing against `reporter`'s `on_finding` trigger (re-scans the whole list on every new finding — cost grows with every finding reported) and an `interval_sec` trigger (steady cost regardless of need). On-demand, mirroring `retester`'s trigger-less pattern, was chosen: the user controls when it runs and what it costs.
- **No automatic mutation of findings.** This review is advisory. It never changes a finding's severity, status, or asset bindings, and it never merges findings in the database. A human reads the report and decides. (Anti-goal: silently acting on a judgment call the human hasn't seen would undermine the exact trust problem this feature exists to fix.)
- **No new evidence-gathering tools.** The review agent reuses tools that already exist and are already bound to other agents (`list_task_findings`, `get_task_node_detail`, `get_task_worker_trace`, `list_task_worker_traces`, `search_task_worker_traces`, `get_task_graph`, `get_finding_traffic`). `list_task_findings` already returns `traffic_count` per finding in one call (confirmed by reading `agent/tools.go`'s `listFindings()` — `PopulateFindingTrafficIDs` populates it before the tool returns), so the evidence-completeness pass does not require one tool call per finding.
- **No structured per-finding tagging in the database.** The output is one Markdown report per run (free text), not a new schema of "duplicate group" or "needs evidence" flags on each finding row. This was an explicit, discussed tradeoff: structured tags would let a findings-list UI filter/sort on them, but that is materially more scope (new columns, migration, UI work) for a capability that is still unproven in regular use. Revisit as a follow-up once this is used a few times.
- **No "revive abandoned intents" capability.** The self-assessment also surfaced a distinct gap (a promising lead blocked by a since-invalidated assumption, never revisited) — that is a separate direction the user deliberately chose not to bundle into this design.
- **No new UI.** Triggered and read back via the HTTP API only, exactly as the user has been operating this whole system during this session (curl + a bearer token). A UI affordance (a button on the findings page, like retester has) is a natural, separate follow-up.

## Constraints

- Mirror `db/finding_retests.go` / `server/finding_retests.go` as closely as the shape of the problem allows: same immutable-record-per-run pattern, same conversation-backed agent session mechanism (the `conversations` / `conversation_activities` tables already used by `retester`), same no-trigger seeding pattern (`seedFindingRetester`'s shape), same advisory-lock-guarded idempotent seed migration.
- The review is scoped to one task per run (matches "Hướng A" as discussed; no cross-task analysis).
- Concurrent review requests for the same task must not spawn two sessions — mirror `CreateFindingRetest`'s row-lock-and-return-existing-active-run behavior, scoped by `task_id` instead of `finding_id`.

## Decided (my calls, made explicit so they can be checked)

- **New agent key: `triager`**, `role='assistant'`, `builtin=false` (editable/deletable like `retester`), created via an idempotent seed function gated by its own settings flag (e.g. `task_submission_review_seed_v1`), following `seedFindingRetester`'s exact transaction/advisory-lock pattern.
- **New table `task_submission_reviews`**, mirroring `finding_retests`' columns minus the single-finding-specific ones:
  `id, task_id, conversation_id, status, summary, error, created_at, started_at, finished_at`.
  No `verdict` enum column (unlike `finding_retests`'s `reproduced|fixed|inconclusive`) — this review doesn't resolve to one of a few fixed outcomes, it produces a free-form report. No `snapshot` column either: unlike a single finding's point-in-time snapshot, `list_task_findings` already reads the live findings whenever the session calls it, and a stale snapshot of 26+ findings has no clear use here.
- **Tools bound to `triager`** (all pre-existing, zero new read tools): `list_task_findings`, `get_task_node_detail`, `get_task_worker_trace`, `list_task_worker_traces`, `search_task_worker_traces`, `get_task_graph`, `get_finding_traffic`.
- **One new write tool, `record_submission_review`**: a single required `summary` string parameter (Markdown). Deliberately not split into structured fields (`duplicate_groups`, `evidence_gaps`, `severity_risks` as separate arrays) — the exact unprompted output we already have from the live main agent (duplicate groups as a table, evidence gaps as a table, a numbered list of abandoned/under-explored leads, a prioritized action list) is already well-organized as Markdown prose; forcing it into a rigid schema now, before the shape of a "good" review has stabilized over a few real runs, would be premature structure.
  - **Size cap: 128KB** (not `finding_retests`'s 16KB `summary` cap). This single field carries everything a full task review produces — the live reference example (26 findings: a duplicate-groups table, an evidence-gaps table, ~15 numbered under-explored leads, a prioritized action list) was already several KB, and a larger task could exceed 16KB easily. 128KB matches `finding_retests`'s `evidence` field cap, which is the closer precedent in both role and size.
  - **Callable more than once per session, not write-once.** `record_finding_retest_result` locks in a verdict and refuses a second, different call — appropriate for a single yes/no conclusion. This review is a long session (read `list_task_findings`, then spot-check several individual findings) that could be interrupted partway; a wrong guess here (write-once) means an interruption loses the whole analysis. Instead: while `status='running'`, each call **overwrites** `summary` (a checkpoint); the row is only locked once the session finishes successfully and `status` moves to `done`. This is a smaller guard than `finding_retests`' identical-or-refuse check, not a new concept.
- **Seeding correction (caught by the builder-check review):** `db.SeedTool` is `ON CONFLICT (key) DO NOTHING` — it only sets a tool's `agents` binding on first insert ever. The 7 pre-existing read tools `triager` needs (`list_task_findings`, `get_task_node_detail`, etc.) already have rows, seeded earlier for `reporter`/`auto` — calling `SeedTool` on them again the way `seedFindingRetester`'s loop does for its own brand-new tools would silently no-op, and `triager` would end up with none of them bound. The seed function must call `AddAgentToToolBinding("triager", [...7 existing keys...])` (additive, idempotent) for those 7, and reserve `SeedTool` for the genuinely new `record_submission_review` only.
- **Initial message to the `triager` session** (mirroring `FindingRetest.InitialMessage()`): names the task id, asks for exactly the three dimensions from the Goals section, and explicitly tells it to use `list_task_findings` first for the traffic_count/severity overview before spot-checking specific findings with `get_finding_traffic`/`get_task_node_detail` — the same order the live main agent actually used.
- **Optional per-run `notes`** (confirmed by the user, mirroring `startFindingRetest`'s `notes` field exactly): `POST /api/tasks/{id}/findings/review` accepts an optional `notes` string, capped the same as retest's (≤4000 chars), appended to the initial message exactly as `FindingRetest.InitialMessage()` appends its own `notes` ("Additional notes for this review:\n" + notes) — lets a human steer one specific run (e.g. "focus on the SSRF findings" or "skip the informational-risk ones") without changing the fixed three-dimension default for every other run.
- **API surface** (mirroring `finding_retests`' handlers exactly):
  - `POST /api/tasks/{id}/findings/review` (body: `{"notes": "..."}`, optional) — starts a review (or returns the already-running one for that task, same dedup-by-lock behavior as `startFindingRetest`).
  - `GET /api/tasks/{id}/findings/reviews` — history, newest first.
  - `GET /api/tasks/{id}/findings/reviews/active` — the small polled status payload (mirrors `ListActiveFindingRetests`, filtered to this task).

## What's left to the builder

- The exact prompt text for the `triager` agent's system prompt (`agent.TriagerDefaultPrompt`, following the existing `agent.RetesterDefaultPrompt` / `agent.ReporterDefaultPrompt` constants' style and location) — the Goals/Decided sections above give its required content, not its wording.
- Exact SQL migration formatting for `task_submission_reviews` (follow `db/schema.sql`'s existing house style: `CREATE TABLE IF NOT EXISTS`, foreign key to `tasks`/`explorations` with `ON DELETE CASCADE`, matching indexes to `finding_retests`' table where analogous).
- Whether `finishRetest`'s pattern (a dedicated `finishReview(id, status, reason)` helper called from the conversation runner's completion path) needs a parallel `finishTaskSubmissionReview`, and exactly how a `triager` conversation's completion is detected (this should follow whatever general mechanism ends a `retester` conversation — the builder should trace that mechanism rather than inventing a new one).
- Whether `GET /api/tasks/{id}/findings/reviews/active` is a new endpoint or an additional filter on an existing active-conversations endpoint — the builder should check for an existing generic "list active conversations for agent X" path before adding a new one.
