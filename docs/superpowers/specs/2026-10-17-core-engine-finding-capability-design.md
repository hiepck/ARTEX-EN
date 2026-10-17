# Core engine finding-capability improvements — design

Status: draft, pending human approval.
Owner request: "cải thiện core engine ... để tool tìm bug được nhiều hơn", explicitly
scoped to directions #2 (burn-signal visibility), #3 (stateful HTTP tool), #4
(reachability/privilege graph edges), #5 (skill binding fix + exotic-class expansion).
Direction #1 (independent validator stage) was explicitly declined by the owner for
this round.

## Intent and why

ARTEX-EN's planner/worker loop is already structurally sound (depth-over-coverage,
route-diversity, serial-chain dispatch, strict anti-hallucination finding discipline —
see prior research, not repeated here). The gap against published autonomous-pentest
research (XBOW, MAPTA, MazeRunner, PentestGPT-v2, EnIGMA) is not reasoning quality, it's
**missing signal and missing tooling**:

- The planner cannot see how much a running branch has already cost (tool calls /
  tokens / wall-clock) — it can only rely on a qualitative "re-check at most once"
  prompt heuristic, and a blunt per-run `max_turns`/`run_seconds` ceiling that fires
  only after the budget is already spent.
- The asset/finding graph has no notion of reachability or privilege scope, so nothing
  in the engine can notice "this new finding changes what's reachable" and
  automatically re-prioritize — chaining is entirely manual/LLM-judged today.
- The worker's only exploitation tools are raw `Bash` (curl/sqlmap/etc., stateless) and
  a GET-only `WebFetch`. There is no session-aware HTTP client, so multi-step
  auth/session-dependent testing (the literature's named weak point for LLM pentest
  agents) is harder than it needs to be.
- The `skills/hunt-*` packs built in prior work (tradecraft per vuln class) were never
  actually verified to be registered into a running ARTEX instance — the setup scripts
  only bind-by-name, assuming the skill directory already exists where the server looks
  for it, and nobody has confirmed that's true end to end.

The owner's standing concern for this round: **do not ship a feature that quietly makes
the tool worse** (regresses existing finding quality/rate to gain a new capability). The
research pass surfaced a concrete structural reason this risk is real here: **this repo
has no integration test that exercises a planner round + worker dispatch + tool
execution end to end.** `go test ./...` is clean (13/14 packages; the 14th package's 22
failures are pre-existing missing-`ARTEX_PG_DSN` environment issues in this sandbox, not
logic failures), but that only proves units behave in isolation — it says nothing about
the loop's emergent behavior, which is exactly what these four features touch.

## Goals

1. Give the planner visibility into per-intent resource burn (tool calls, tokens,
   elapsed time) so it can act on existing `kill_work`/`steer_work` primitives before a
   hard cap trips, instead of after.
2. Add a reachability/privilege edge concept to the graph, and make the planner able to
   actually see it (today it can't see relation *types* on even the edges that already
   exist).
3. Give the worker a stateful, session-aware HTTP tool (cookie jar, full method/header/
   body control) as a first-class, opt-in tool, routed through the same recording proxy
   as every other tool so evidence capture doesn't regress.
4. Confirm (and fix if broken) that the already-authored `skills/hunt-*` packs are
   actually discoverable and bindable in a real running ARTEX instance, audit the
   per-agent skill-description character budget before adding more, and then author the
   7 deferred exotic-class skills (prototype pollution, mass assignment, deserialization,
   SSTI, XXE, CORS, subdomain takeover) in the existing pattern.
5. Build a minimal integration-test harness (fake LLM provider + fake HTTP target + real
   test Postgres) that drives one planner round → intent dispatch → worker execution →
   tool call → graph write-back, and asserts on resulting graph state — because none
   exists today and all four features above change planner/worker-visible behavior.
6. Every behavior change must be reachable through an existing or new opt-in toggle
   (the `settings` table / `tools` table pattern already used by `noa_compaction`,
   `agent_traffic_binding`, and `defaultUnbound` tools), so an operator who does nothing
   sees zero behavior change.

## Non-goals

- Direction #1 (independent validator/confirmation stage) — explicitly out of scope for
  this round.
- Rewriting the planner/worker prompts' core strategy (depth-over-coverage, route
  diversity, etc.) — this design only adds *visibility* and *tooling*, it does not
  change the planner's existing decision-making prose except to tell it the new signals
  exist.
- A full price/dollar-cost model for the burn signal — raw tool-call count / token count
  / elapsed seconds is sufficient for v1; a $-cost conversion is future work (see the
  existing `research_notes/.../artex_optimizations.md` cost audit, which found no
  pricing table anywhere in the codebase — adding one is a separate project).
- A general-purpose graph query language or UI for reachability edges — v1 only needs
  the edges to exist and to be visible to the planner's existing tools (`graph_overview`,
  `node_detail`), not a new exploration UI.
- Replacing `Bash`/`curl` for exploitation — the stateful HTTP tool is additive, for
  cases where session continuity matters; it does not deprecate raw shell access.
- Live-lab (real vulnerable target) end-to-end validation. The owner did not have one
  available at design time; the synthetic integration harness (goal 5) is the required
  regression gate. If a lab becomes available later, running these features against it
  is a valuable follow-up, not a blocker for this plan.

## Anti-goals (would make this a failure even if it technically works)

- A feature that is impossible to turn off, or that changes behavior for an agent that
  never opted in.
- A schema change that is not idempotent/additive (this repo's house style: `schema.sql`
  re-applies on every boot; every existing change is `ADD COLUMN IF NOT EXISTS` / `CREATE
  TABLE IF NOT EXISTS` / a `DO $$ ... constraint-recreate ... END $$` block — no
  exceptions here).
- Adding skill content that silently exceeds the 8000-char/500-char-per-skill meta-tool
  budget (`norma/skill/skill.go`), causing an earlier-bound skill to go invisible to the
  model with no error or log. Silent capability loss is explicitly worse than not
  shipping the feature.
- A stateful HTTP tool whose traffic bypasses the recording proxy (breaks the evidence
  chain that findings/reports depend on).
- Shipping graph-schema changes (reachability edges) without also updating
  `graphOverviewData`/`compactIntents`/`node_detail` to render them — an edge the planner
  can't see is dead weight that only adds migration risk for zero benefit.

## Constraints (established by research, not up for relitigating)

- **Schema style**: additive only, following `db/schema.sql`'s existing idempotent
  convention. See `db/schema.sql:166,196,202-203,205-241,263-275,415-451` for the
  `ADD COLUMN IF NOT EXISTS` / enum-recreate pattern to copy.
- **Feature-flag template**: copy the `noa_compaction` (`server/manager.go:277,498-505`)
  or `agent_traffic_binding` (`server/manager.go:253`) pattern — a `settingX` constant, a
  `Manager.XEnabled()/SetX()` pair backed by the generic `settings` table
  (`db/schema.sql:357-361`), consulted lazily at the call site. For the stateful HTTP
  tool specifically, use the **tools-table opt-in** pattern instead (`db/tools.go:12-23`,
  `Enabled`/`Agents` columns, `defaultUnbound` precedent at `agent/toolcatalog.go:61`) —
  this is the mechanism that already exists for exactly "add a new tool without it
  affecting agents that don't bind it."
- **Tool definitions**: use `actool.Build(actool.Spec{...})` via the existing
  `readTool`/`writeTool` wrappers (`agent/tools.go:280-300`); tool params are parsed with
  plain `json.Unmarshal` into a Go struct and neither the SDK nor the LLM providers
  enforce `additionalProperties: false`, so adding optional fields to existing tool
  schemas (`report_finding`, `insert_assets`) is safe and backward compatible.
- **Recording-proxy wiring**: any new tool that issues HTTP requests must route through
  it or evidence capture silently breaks for that tool. Copy `server/customtool.go:459-
  485` (`httpProxyTransport`) for the `Transport{Proxy, TLSClientConfig.RootCAs}` setup —
  this is the closest ready-made template; it is not currently stateful (fresh client per
  call, no `Jar`), so the new tool is the first consumer to add `net/http/cookiejar`.
- **Planner visibility is not automatic**: `graphOverviewData()`/`compactIntents()`
  (`agent/tools.go:354-558`, `20-49`) decide what the planner sees. Both the burn signal
  and the reachability edges require explicit changes here, not just a DB write — this
  was confirmed as the single most important technical finding of the feasibility pass
  (edges already exist for `exploration_edges`, e.g. `RelYields`/`RelDerivedFrom`, and
  are *still* invisible to planner reasoning today because only bucket membership, not
  relation type, is serialized).
- **No regression baseline exists today** beyond unit tests — `go test ./...` passing is
  necessary but not sufficient evidence of no regression for any of these four features,
  because none of them is exercised by a loop-level test.

## Decisions made (the "how", as decided)

1. **Build order**: integration harness first (prerequisite for everything else,
   enumerated as its own workstream below), then the skill-binding fix (cheapest, lowest
   architectural risk, unblocks confirming whether prior work even functions), then burn
   signal, then reachability edges, then the stateful HTTP tool (fully independent of the
   other three, can run in parallel once the harness exists).
2. **Reachability model**: a new, purpose-built `asset_edges(src_asset_id, rel,
   dst_asset_id, ...)` table, not an extension of `exploration_edges`'s enum. Reasoning:
   `exploration_edges` models causal/reasoning relations between graph nodes
   (goal/intent/fact/finding/hint/digest); infrastructure reachability is a different
   axis (asset-to-asset) and overloading the reasoning enum would conflate the two.
   `CHECK (rel IN ('can_reach'))` for v1 (see decision 7 — `scoped_to` dropped from v1).
   More relation kinds are additive later.
7. **`asset_edges` scoping, writers, uniqueness, and the dropped `scoped_to` value**
   (settled after a builder-check pass surfaced these as under-specified; owner should
   skim before approving, but a wrong call here is reversible, unlike decision 8 below):
   - **Scoped per task/exploration, not global per physical asset.** Add an
     `exploration_id BIGINT` (or `task_id`) column to `asset_edges`, indexed, and filter
     by it in the render path. Reasoning: `assets` rows are reused across
     tasks/explorations for the same company (`assets.task_ids BIGINT[]`,
     `db/schema.sql:52,95`), so an un-scoped edge discovered in one engagement would
     become silently visible to every other exploration sharing that asset row —
     cross-engagement leakage, not just noise. Scoping is the conservative default and
     matches how the rest of the graph (tasks ↔ exploration_id, 1:1 per
     `db/schema.sql:514`) is already partitioned.
   - **Both `report_finding` and `insert_assets` can write an edge**, through one shared
     internal helper, not two independent implementations. Reasoning: sometimes the
     destination asset already exists in the graph (finding-time edge, via
     `report_finding`'s existing `asset_ids`), sometimes the reachable asset is newly
     discovered by the same action that proves reachability (needs creation first, via
     `insert_assets`-style logic) — supporting only one tool would leave some real
     discoveries with no way to be recorded.
   - **`(src_asset_id, rel, dst_asset_id, exploration_id)` is unique; writes upsert**
     (`ON CONFLICT DO NOTHING` or bump an `updated_at`), not append-only. Reasoning: the
     same re-report pattern the planner prompt already guards against for intents
     ("re-check at most once") will otherwise produce duplicate edges that either need
     read-time dedup in `graphOverviewData` anyway, or get seen by the planner as N
     independent signals instead of one.
   - **`scoped_to` is dropped from the v1 CHECK constraint.** Nothing in this design
     allocates a workstream to populate it (it would need a backfill/sync job against
     the existing `company_scope`/asset-provenance data, `db/schema.sql:47-51`, which is
     out of scope here) — shipping an enum value nothing ever writes is dead weight per
     the anti-goals. Only `can_reach` ships in v1; `scoped_to` (or a credential-scope
     concept generally) is explicitly future work if a populating workstream is
     scoped later.
8. **Evidence bar for writing a `can_reach` edge: evidence-gated (owner-decided).** A
   `can_reach` edge must carry the same proof discipline `report_finding` already
   enforces (`traffic_refs`/`evidence_hint_id`, "don't guess IDs, don't infer from
   domain/time alone" — `agent/tools.go:1228-1234`), reused rather than reinvented for
   this second tool surface. Rationale (owner's call, made explicitly against option (b)
   unevidenced-metadata): a hallucinated edge must not be representable at all, so it
   cannot repeatedly redirect planner attention toward a phantom route — directly the
   "quietly makes it worse" failure mode the anti-goals section exists to prevent. This
   is more work than (b) (the evidence-binding plumbing now serves two tool surfaces
   instead of one) but was judged worth it given this design's standing regression
   concern.
3. **Burn metric v1 is read-side only, no new columns.** Compute `{tool_calls, tokens,
   elapsed_s}` per running/open intent from the existing `activity` table at
   `graph_overview` call time (one grouped query, same cost class as the
   `TokenStatsBySession`/`CountFinishedIntents` queries already run every round). A
   cached/precomputed column or view is explicitly deferred — only build it if read-time
   aggregation proves too slow in practice, which is a thing the integration harness (or
   later production telemetry) can tell us, not something to presuppose.
4. **The stateful HTTP tool is one tool with sub-actions** (method/url/headers/body/
   cookie-reset params), not separate GET/POST/etc. tools — mirrors the existing
   `Spec.IsReadOnly(input)` capability (per-call read-only determination, e.g. GET/HEAD
   read-only, POST/PUT/DELETE not) rather than needing N tool definitions.
5. **Skill-budget audit is a required step before authoring new skills**, not optional
   polish: sum the description-character cost of every skill currently bound (by any
   setup script) to `worker`/`planner`/`pentest`, compare against the 8000-char total /
   500-char-per-skill budget (`norma/skill/skill.go`'s `maxSkillListingChars`/
   `maxSkillDescChars`), and trim any oversized description (e.g. `hunt-idor`'s, sampled
   at ~1400 chars) *before* adding the 7 new exotic-class skills, so a new skill doesn't
   silently push an older one past the budget into name-only.
6. **All four features ship behind their own independent toggle** — not one umbrella
   flag — so each can be enabled/rolled back independently, matching how
   `noa_compaction` and `agent_traffic_binding` are already independent of each other.

## Workstreams (what gets built)

### W0 — Integration test harness (prerequisite)

Compose the existing fake-LLM-provider pattern (`agent/provider_capture_e2e_test.go`,
scripted Anthropic/OpenAI-format SSE responses via `httptest.NewServer`) with a real test
Postgres (same requirement the existing `TestFindingWorkflow*`/`TestNotify*` tests already
have) and a fake HTTP target server, to drive: engine spawn → one planner round (via the
fake LLM scripted to call `add_intent`) → worker claims the intent → worker calls a tool
(scripted to call the fake target) → `record_fact`/`report_finding` → assert on resulting
`exploration_nodes`/`activity` rows. This becomes the regression gate for W1-W3 below (W4
needs its own skill-loading-specific verification, see W-Skill).

### W-Skill — Verify + fix skill binding, then expand

1. Stand up (or use an existing dev) ARTEX instance built the standard way (`go build
   -tags embedui -o artex ./cmd/artex` from repo root, per README), confirm `BaseDir()`
   resolves to the repo root so the default `skillDir` is this repo's `./skills/`, and
   confirm `GET /api/skills` (or equivalent) lists the already-authored `hunt-*` packs.
2. If the setup scripts' bind-by-name calls (`PUT /api/agents/{agent}/visibility`) are
   silently matching nothing (no directory found), fix the root cause (document the
   required `ARTEX_SKILL_DIR`/working-directory assumption, or fix path resolution) —
   whichever the investigation in step 1 shows is actually broken.
3. Audit total bound-skill description bytes per agent (decision 5 above); trim any
   skill whose frontmatter `description` is unnecessarily long.
4. Author `skills/hunt-prototype-pollution/`, `hunt-mass-assignment/`,
   `hunt-deserialization/`, `hunt-ssti/`, `hunt-xxe/`, `hunt-cors/`,
   `hunt-subdomain-takeover/` SKILL.md (+ references where warranted), following the
   `hunt-ssrf` precedent exactly (depth-floor ≥25 attempts, evidence gate, stop-authority
   disclaimer owned by `hunting-methodology`), with the lint pass
   (`skills/chain-toolkit/scripts/tests/_lint_skill.py`) and a `setup-enhance-phase3.sh`
   + its test, matching phase1/phase2's structure.

### W1 — Burn-signal visibility

1. New `ExplorationStore` method (sibling to `TokenStatsBySession`) returning
   `{tool_calls, tokens, elapsed_s}` grouped by intent id, filtered to currently
   open/running intents for one exploration.
2. Thread it through `graphOverviewData()` into `compactIntents()` as an added field per
   running/open intent entry.
3. Update the planner's prompt (`plannerDefaultTmpl`) to document the new field and give
   it explicit guidance on when burn-without-progress should trigger `steer_work`/
   `kill_work` — grounded in MAPTA's empirical thresholds (~40 tool calls / ~$0.30 /
   ~300s correlating with failure) as a starting reference point, not a hard rule, since
   ARTEX's intent granularity differs from MAPTA's.
4. Gate behind a new setting, e.g. `burn_signal_enabled` (default off), following the
   `noa_compaction` template.

### W2 — Reachability/privilege graph edges

1. New `asset_edges` table (additive, `CREATE TABLE IF NOT EXISTS`): `src_asset_id`,
   `rel` (`CHECK (rel IN ('can_reach'))`, v1 only — see decision 7), `dst_asset_id`,
   `exploration_id` (scoping, decision 7), unique on
   `(src_asset_id, rel, dst_asset_id, exploration_id)` with an upsert write path.
2. Shared internal helper writable from both `report_finding` and `insert_assets`
   (decision 7), plumbed through the same evidence-binding discipline `report_finding`
   already uses (decision 8: evidence-gated) — no edge can be written without a
   `traffic_refs`/`evidence_hint_id`-equivalent proof reference.
3. Update `graphOverviewData()`/`node_detail` to render the new edge type to the planner
   — required, not optional (see anti-goals).
4. Planner prompt update: when a finding/asset carries a new reachability edge, the
   planner should query what's newly reachable and consider it as a candidate new route
   (ties into the existing "route diversity" strategy already in the prompt).
5. Gate behind a new setting, e.g. `reachability_edges_enabled` (default off).

### W3 — Stateful HTTP tool

1. New tool (new file, e.g. `agent/tools_http.go`) built via `actool.Spec`, closing over
   a per-worker-run `http.Client` with `cookiejar.New(nil)` and the same
   `Transport{Proxy, TLSClientConfig.RootCAs}` setup as `httpProxyTransport`
   (`server/customtool.go:459-485`), constructed once per `Worker.execute` call so
   cookies persist for the run and reset on the next intent.
2. Register in `agent/toolcatalog.go`'s `builtinToolsByAgent()["worker"]`, seeded
   **default-unbound** (the `goal_met` precedent, `agent/toolcatalog.go:61`) so it is
   strictly opt-in per agent via the existing `tools` table / UI, with zero effect on any
   agent that doesn't explicitly enable it.

## What's left to the builder

- Exact Go struct/field names, SQL migration text, and prompt wording for each
  workstream — the design fixes the *shape* and the *file locations*, not every literal.
- Whether W1's burn query is a single grouped SQL query or two (tool-call count vs.
  token/time) — whichever is simpler to implement correctly against the existing
  `activity` table structure.
- The exact MAPTA-derived numeric guidance given to the planner in W1 step 3 (a
  suggested threshold vs. no number at all, just "compare against your own judgment of
  normal progress") — pick whichever reads better in context next to the existing
  prompt's "at most one re-check" instruction.
- Whether W-Skill's root-cause fix (if the binding is in fact broken) is a doc fix, a
  setup-script fix, or a server-side default-path fix — depends entirely on what step 1's
  investigation finds; this design intentionally does not presuppose which.
- Sequencing/parallelization across W1-W3 once W0 exists — the design states a
  reasonable default order (harness → skill fix → burn → reachability → HTTP tool) but
  the builder may reorder based on what's actually blocking what once work starts.
- Test coverage depth for each workstream beyond "the W0 harness exercises it at least
  once end to end" — additional unit tests per workstream are expected but not enumerated
  here.
