# ARTEX Chaining + Bug-Hunting Pack — Design Spec

**Date:** 2026-10-11
**Status:** Design (pre-implementation)
**Scope:** Add bug-finding depth and multi-bug exploit chaining to ARTEX **only** through
skills, a custom agent, tool bindings, and shipped scripts. No changes to the `norma`
harness, the engine, the DB schema, or any core Go behavior.

---

## 1. Goals

1. ARTEX finds **more** bugs per engagement (deeper per-class methodology, higher attempt
   floors, source-review greps, bypass tables).
2. ARTEX **chains** discovered bugs into higher-impact composite findings (red-team flavor,
   strictly defined here as: correlate existing findings and actively pursue new hops that
   escalate a confirmed bug toward a crown-jewel impact such as ATO / RCE / mass data access).
3. Maximum reuse of ARTEX's **existing** built-in features (planner/worker loop, finding
   workflow, exploration graph, traffic-evidence pipeline, scope gate, skills, agent triggers).
4. Low false-positive rate preserved via per-hop evidence gates and an adversarial validation
   discipline.

## 2. Constraints & Non-Goals

**Hard constraints (from the user):**

- **No harness changes.** `github.com/Autumn-27/norma` is off-limits.
- **No core-Go changes.** The pack is additive: files under `skills/`, agent configuration
  applied over the existing REST API, and scripts invoked via `Bash`. Target is **zero new Go**.
- Honor the translation-fork rules in `CLAUDE.md` (English-only user-facing text; module path
  unchanged; anything checked in stays in sync-friendly form).

**Non-Goals (YAGNI — explicitly deferred):**

- Detection-gap / MITRE ATT&CK mapping output. Deferred; chaining + impact first.
- A separate chain state store (e.g. penred's SQLite). Chain state lives in ARTEX's own
  PostgreSQL finding store (option **a**, §6).
- A native Go `report_chain` tool. Rejected for v1 (see §6.3); the existing `report_finding`
  plus the exploration graph fully express a chain.

## 3. Ground Truth — confirmed ARTEX mechanics

This design rests on mechanics verified by reading the code, not assumptions:

- **Per-agent skill loading.** `server/assembly.go` `wireAgentAugment` loads skills from the
  skills directory via `skill.LoadDir(skillDir)` and packs **only the skills bound to an agent**
  (`pg.AgentSkillNames`) into that agent's Skill meta-tool. Skills live in `skills/<name>/SKILL.md`
  with optional `references/` and `scripts/` (confirmed by `server/platform_tools.go`).
- **Custom agents are first-class and config-driven.** REST surface (`server/server.go`):
  `POST /api/agents`, `PATCH /api/agents/{key}`, `PUT /api/agents/{key}/prompt`,
  `PUT /api/agents/{key}/visibility`, `PUT /api/agents/{key}/config`, `PUT /api/tools/{key}`
  (per-agent tool binding), `POST /api/skills` + `PUT /api/skills/{name}/files/{file...}`.
- **Generic P3 trigger system** (`db/triggers.go`, `server/triggers.go`): a custom agent can
  subscribe to six conditions — `on_finding`, `on_goal_met`, `on_task_timeout`, `on_tool_call`,
  `on_task_create`, `interval`. **`on_finding` is the "on new finding" trigger** this design uses.
  Triggers are **custom-agents-only** (`a.Builtin` is rejected). `StartTriggeredRun`
  (`server/conversations.go`) fires the agent with task context (`taskID`, `taskDesc`, `taskGoal`)
  and a configurable serial/parallel + merge policy.
- **Chaining already exists at the prompt level.** The planner (`agent/planner.go`, serial-chain
  guidance) and worker (`agent/promptcatalog.go`) already handle serial exploit chains step by
  step via `TodoWrite`. The pack **strengthens** this; it does not invent it.
- **Finding workflow.** `report_finding` registers a finding (standalone `finding_id` +
  exploration `node_id`); the hardwired `reporter` agent is invoked to write the Markdown report
  (`agent/finding_workflow.go`, `agent/promptcatalog.go` `ReporterDefaultPrompt`). Traffic
  evidence binds via `bind_finding_traffic`; `retester.go` reproduces. The `reporter` prompt is
  reporting-only ("do not perform penetration tests and do not exploit anything").
- **Finding store schema** (`db/schema.sql`): `findings` has `vulnclass`, `name`, `severity`
  (critical|high|medium|low), `summary`, `evidence`, `report` (free-form Markdown), `node_id`
  (into the exploration graph), `asset_ids`, `status`. No chain column exists — and none is needed.

## 4. Architecture

Three additive components.

### 4.A Skills pack (content → `skills/`)

Skills in penred's proven format (YAML frontmatter + sections + lazy-loaded `references/`).
Ported/adapted from the user's `~/Tools/penred`:

| Skill dir | Purpose |
|---|---|
| `hunting-methodology` | Master find-more-bugs: 5-phase flow + depth-matrix floor (≥25 attempts/class). |
| `chain-hunting` | Master chaining: hybrid correlate→chain-table→pursue→gate→compose. Ships `references/chain-table.md` (capability→next-bug→terminal-impact tables, deep-chain examples) and `references/payloads.md` (bypass tables). |
| `hunt-idor`, `hunt-ssrf`, `hunt-oauth`, `hunt-xss`, `hunt-upload-rce`, `hunt-business-logic`, `hunt-info-disclosure` | Per-class methodology + `references/` incl. source-review greps. |
| `validation-gate` | 7-Question Gate, never-submit list, Owned-Test-Account Trap, devil's-advocate stance. Ships `references/never-submit.md`. |
| `chain-toolkit` | Hosts the Python scripts (§4.C) and documents how to call them from `Bash`. |

**Binding:** `hunting-methodology` + the `hunt-*` skills + `validation-gate` bind to **worker**
(and optionally **planner** for the methodology/chain overview). `chain-hunting` +
`chain-toolkit` + `validation-gate` bind to the **chainer** agent (§4.B). Binding is done over
`PUT /api/agents/{key}/visibility`.

### 4.B Chainer custom agent + `on_finding` trigger

A new **custom** agent `chainer`, created and configured entirely over the REST API:

- **Role:** correlation + chain planning brain. On every new finding it reads the finding set and
  the exploration graph, ranks candidate next-hops against the chain-table, and — for an in-scope
  candidate — **plants a chain-pursuit hint** into the task graph (`add_task_hint`) describing the
  capability gained, the target impact, and the specific next hop to attempt.
- **It does not exploit directly.** Active pursuit flows through the existing planner→worker loop:
  the planted hint is a graph change that the planner picks up on its next round and dispatches to
  a worker, whose tool calls already pass the scope gate (`intercept`/`guard`). This keeps the
  safety boundary exactly where ARTEX already enforces it. (Alternative — giving `chainer` raw
  exploit tools — was rejected: it would duplicate the worker's role and move exploit traffic
  outside the established worker scope-gate path.)
- **Trigger:** `POST /api/agents/chainer/triggers` with `on_finding=true` and a `finding_message`
  that instructs the correlate→rank→plant flow. Run policy: serial, merge `by_task` (one chain
  analysis per task at a time; coalesce bursts of findings).
- **Tools bound:** correlate/plan tools — `list_findings`, `list_task_findings`,
  `get_task_node_detail`, `get_task_graph`, `list_task_worker_traces`, `get_task_worker_trace`,
  `add_task_hint`, `record_fact`, `Bash` (for the toolkit), the Skill meta-tool, **and**
  `report_finding` used **only** to register the composite chain finding (§6). **Leg** findings are
  reported by workers, not the chainer; the chainer registers the composite once a terminal impact
  is proven across the legs. The chainer has **no raw exploit tools** — it never drives traffic at
  the target.
- **Prompt:** the chain-hunting methodology, the hybrid rule, the per-hop evidence gate, the
  chain-confidence rule (min over legs), and the hard scope rule (§9).

The hardwired `reporter` agent is left reporting-only, with one small additive prompt note: when
it notices related findings while writing a report, it may record a chain-candidate hint. This is
optional redundancy; the primary chain trigger is the `chainer` `on_finding` subscription.

### 4.C Chain toolkit (Python via `Bash`)

Scripts shipped inside `skills/chain-toolkit/scripts/`, called by the chainer via `Bash`
(`python3` is present in the ARTEX runtime). **Stateless, JSON-in / JSON-out** — no private DB,
consistent with option (a):

- `chain_plan.py` — input: findings JSON (exported by the agent from ARTEX via its read tools);
  output: ranked candidate next-hops from the chain-table.
- `cvss_calc.py` — input: vulnclass + chain metadata; output: CVSS 4.0 vector + score → severity
  bucket (critical|high|medium|low) for the composite finding.
- `dedup.py` — input: a finding/chain text + a corpus; output: near-duplicate verdict (simhash),
  to suppress duplicate legs and duplicate composites.

Scripts run with `python3 -I` and receive data only as arguments/stdin (never imported from an
untrusted CWD). Script failure or timeout is non-fatal: the chainer treats "no suggestion" and
continues with its own reasoning (fail-safe; the run never aborts on a toolkit error).

## 5. Data flow (end-to-end)

```
Hunt (hunting-methodology + hunt-* skills) → worker finds a bug
  → validation-gate (7-Q, owned-account trap) passes
  → worker: report_finding(leg, confirmed)  [+ bind_finding_traffic evidence]
      → on_finding trigger → chainer agent wakes (task context)
          → reads findings + graph; chain_plan.py ranks next-hops
          → in-scope candidate? → add_task_hint("capability X → pursue hop Y → impact Z")
              → planner picks up the hint next round → dispatches worker
                  → worker attempts hop (scope gate on every tool call)
                      → hop proven (independent postcondition + traffic evidence)?
                          yes → report_finding(next leg); capability updates; loop
                          no  → mark theoretical; drop (not chainable)
          → terminal impact reached → cvss_calc.py (chain CVSS 4.0) + dedup.py
              → report_finding(vulnclass="chain", severity=chain-level,
                               summary=chain narrative, evidence=per-leg proof + leg finding_ids,
                               traffic_refs=combined)   ← composite finding (option a)
      → reporter agent writes the composite Markdown report (existing workflow)
  → report/ renders the deliverable (existing)
```

## 6. Storage — option (a): composite finding in ARTEX PostgreSQL

### 6.1 Mapping
A chain is a **composite `findings` row**, reusing the existing `report_finding` tool:

- `vulnclass = "chain"` (distinguishes composites from single legs).
- `name` = the chain's terminal impact (e.g. "SSRF → IMDS → account takeover").
- `severity` = the chain-level bucket from `cvss_calc.py` (CVSS 4.0).
- `summary` = one-line chain statement (legs → impact).
- `report` (Markdown, written by the reporter) = the full chain narrative: each leg, the
  capability it granted, the precondition→postcondition link to the next leg, and the referenced
  leg `finding_id`s.
- `evidence` = per-hop proof pointers + the ordered leg `finding_id`s.
- `traffic_refs` = the combined ordered traffic evidence across legs (via the existing
  `bind_finding_traffic` flow).
- `node_id` = the composite's exploration node; the leg findings already live in the same
  exploration graph, so the chain is navigable there without any schema change.

### 6.2 Why no schema change is needed
The `findings` table already carries free-form `report` Markdown, a severity bucket, a graph
`node_id`, and an `evidence` field. Leg findings are ordinary rows; the composite row references
them by id in text and shares the exploration graph. Chain structure is therefore expressible with
existing columns and the existing graph.

### 6.3 Rejected: native `report_chain` Go tool
A dedicated Go tool would only add auto-CVSS and auto graph-edge linking. Both are achievable
without Go: CVSS via `cvss_calc.py`, linkage via the shared exploration graph that leg and
composite findings already occupy. Adding it would violate the zero-Go constraint for no material
gain. **Decision: not in v1.** Reconsider only if, in practice, the text-reference linkage proves
too weak.

## 7. Evidence & validation gates (keep false positives low)

- **Per-hop postcondition must be independently observed.** A credential leg must authenticate a
  *second* request; a status-200 alone is never proof. This mirrors ARTEX's existing
  "adversarially check your own conclusions" worker discipline.
- **Precondition provenance.** Hop B's precondition must point at hop A's *proven* postcondition;
  no orphan hops.
- **Chain confidence = min(leg confidences).** Any single `theoretical` (un-triggered) leg makes
  the whole chain theoretical → **not reportable** as a confirmed chain.
- **Owned-Test-Account Trap** mandatory for IDOR / cross-tenant legs.
- **Dedup** (`dedup.py`) blocks duplicate legs and duplicate composites.
- Existing ARTEX guarantees are retained and relied on: worker reports only what it actually
  triggered this run; `retester.go` reproduction; traffic-bound evidence.

## 8. Severity / CVSS 4.0 & reporting

- Composite severity is computed by `cvss_calc.py` using **CVSS 4.0** (matches Bugcrowd /
  Intigriti / YesWeHack / Immunefi; HackerOne's 3.1 can be emitted as a secondary vector if
  needed). The score maps to the `findings.severity` bucket.
- The composite report is produced by the **existing reporter workflow** — no new report path.
  The chain narrative (legs, capabilities, impact) is carried in `report`/`evidence` and rendered
  by the existing `report/` package.

## 9. Scope safety & error handling

- **Scope is re-checked on every hop.** Worker tool calls pass the existing `intercept`/`guard`
  scope gate on each request. A hop that would pivot **out of scope** is a **hard stop**: the
  chainer marks the chain `blocked-scope` and does not pursue it, without asking. The `guard`
  deny-rules remain the backstop.
- **Fully-auto pursuit** applies **only to in-scope hops** (per the user's decision). No approval
  prompt for an in-scope hop; an out-of-scope hop is never attempted.
- **Toolkit errors** (script failure/timeout) are non-fatal: "no suggestion", continue.
- **Trigger storms:** `on_finding` merge policy `by_task` coalesces finding bursts so chain
  analysis runs once per task at a time.

## 10. Delivery & setup

**Decision:** documented setup **plus a seed script**. Core remains zero-Go.

- `skills/` content is checked into git (version-controlled, sync-friendly English).
- A `scripts/setup-chaining-pack.sh` (or equivalent) seed script applies the runtime agent
  configuration over the **existing REST API**: create the `chainer` agent, set its prompt, bind
  skills (`visibility`), bind tools, create the `on_finding` trigger, and apply the small reporter
  prompt note. One command → reproducible setup.
- A README documents the manual UI equivalent and the required settings (so the config is legible
  without running the script).

## 11. Testing

- **Python scripts:** unit tests (JSON in/out) for `chain_plan.py` ranking, `cvss_calc.py` vector
  correctness, and `dedup.py` near-duplicate detection. Run standalone, outside ARTEX.
- **Skills:** validated by a **manual sample-chain run** on a self-hosted lab (no ARTEX chain-eval
  harness exists). Canonical scenario: `SSRF → cloud metadata → credential → privileged action`,
  verifying the full flow — worker finds the SSRF, chainer plants the hint, planner dispatches the
  next hop, the composite finding is stored with option-(a) structure, and the reporter renders it.
- **No new Go tests** (no Go is added).

## 12. Open items (planning-level, not blockers)

- Exact `Bash`/`python3` invocation form inside `chain-toolkit` (argument vs stdin) — settle in
  the plan.
- Whether `chainer` should also subscribe to `on_goal_met` as a final "any unchained findings?"
  sweep — evaluate during implementation; default off.
- **Load-bearing assumption to verify first in the plan:** that the planner wakes on a hint added
  by `add_task_hint` (a graph change) and dispatches the next hop. If the planner only wakes on
  worker-produced facts, the chainer instead records the next hop as a `record_fact` / intent the
  planner already consumes. Confirm against `agent/planner.go` trigger-event handling before build.
- The precise set of chain-table entries to port first (prioritize the high-yield feeders:
  SSRF, IDOR, info-disclosure, open-redirect, OAuth).

## 13. File manifest (what gets added)

```
skills/hunting-methodology/SKILL.md
skills/chain-hunting/SKILL.md
skills/chain-hunting/references/chain-table.md
skills/chain-hunting/references/payloads.md
skills/hunt-idor/SKILL.md            (+ references/)
skills/hunt-ssrf/SKILL.md            (+ references/)
skills/hunt-oauth/SKILL.md           (+ references/)
skills/hunt-xss/SKILL.md             (+ references/)
skills/hunt-upload-rce/SKILL.md      (+ references/)
skills/hunt-business-logic/SKILL.md  (+ references/)
skills/hunt-info-disclosure/SKILL.md (+ references/)
skills/validation-gate/SKILL.md
skills/validation-gate/references/never-submit.md
skills/chain-toolkit/SKILL.md
skills/chain-toolkit/scripts/chain_plan.py
skills/chain-toolkit/scripts/cvss_calc.py
skills/chain-toolkit/scripts/dedup.py
skills/chain-toolkit/scripts/tests/…
scripts/setup-chaining-pack.sh       (REST-API seed for the chainer agent + bindings + trigger)
docs/chaining-pack.md                (setup + manual-UI equivalent)
```

**No changes** to: `norma` (harness), `agent/` Go, `server/` Go, `db/` Go or `db/schema.sql`,
`cmd/`, build scripts.
