# Stale-Negative Revival Design

## Intent and why

On the user's live task #15, the main agent's self-assessment surfaced a concrete missed High/Critical opportunity: a worker recorded a negative fact ("`getFollowerAssets`/payment endpoints return 403, looks like a seller-state gate") and the planner stopped pursuing that direction. Much later, a different fact (#2704) proved the 403 was actually a **wrong-credential-type** artifact — a fresh session access token sailed through where the old DSR cookie didn't. Nothing in the system surfaced that the earlier "closed" direction was worth revisiting; the connection lived only in the main agent's one-off manual re-read of the whole graph, not in anything the planner sees every round.

Confirmed by reading the live code (not assumed): `agent/planner.go`'s prompt already tells the planner to be skeptical of a *weak* negative fact's own evidence at the moment it is first read (self-recheck once), but has no mechanism for a *later, separate* fact to retroactively flag an earlier, solidly-evidenced negative conclusion as wrong. `graph_overview` tracks intent/fact state and lineage but has no field for "this closed intent's conclusion may now be stale." The only edge types that exist (`RelYields`, `RelDerivedFrom`, `RelProves`, `RelSpawns`, `RelCovers`) have no "contradicts/supersedes" semantic, and `record_fact` has exactly one node-link parameter (`intent_id`, always a single parent intent) — a new fact cannot reference another fact at all.

## Goals

- Let a worker, when recording a fact, explicitly mark that it invalidates one or more earlier facts (`supersedes`), gated so a weak claim can't silently overturn a settled conclusion.
- Surface superseded facts to the planner every round, so it can decide whether the intent(s) that depended on the old conclusion are worth revisiting — using the `add_intent(parent_ids=[...])` mechanism that already exists for deriving a new intent from a fact.
- Default off, like every experimental graph feature shipped this session (`burn_signal_enabled`, `reachability_edges_enabled`).

## Non-goals

- **No automatic re-opening of the old intent.** The planner decides, the same way the existing `asset_reachability` signal "does not create a new goal on its own — it only re-prioritizes." This feature only makes the contradiction visible; `add_intent` (already planner-accessible) is how a new round of work actually starts.
- **No new table.** The existing `exploration_edges` table and `ExplorationStore.Link(from, rel, to)` (already writes an arbitrary-relation edge idempotently) are reused as-is. (Correction from an earlier draft of this doc: `exploration_edges.rel` is **not** unconstrained — `db/schema.sql:257` has `CONSTRAINT ck_edge_rel CHECK (rel IN ('spawns','derived_from','yields','proves','covers'))`. A migration extending this list IS required; see Decided.)
- **Scoped to fact → fact only.** Not finding, not intent, not goal. The problem is specifically "an earlier negative/blocking *fact*'s conclusion turned out to be wrong" — findings are confirmed vulnerabilities (nothing to "invalidate" in the same sense), and intents/goals are not conclusions.
- **No batch-wide re-validation of old facts.** This is a forward-only annotation (new fact → old fact it overturns), not a system that re-checks every existing fact for contradictions.
- **No change to `blocked_reason`.** Confirmed by reading the code: that field is an operational LLM-quota-outage marker, unrelated to this feature; this design does not touch it.

## Constraints

- Mirror `report_finding`'s `reaches` parameter as closely as the shape of the problem allows: evidence-gated (hard-reject a claim with insufficient proof, never silently drop it), feature-flagged the same way every prior experimental graph feature in this project is (`agent.XEnabled func() bool` package var, wired once in `server/assembly.go`, backed by a `Manager.XEnabled()/SetX()` pair over a persisted setting, exposed in both the GET payload and the PATCH handler of `/api/settings` — the GET/PUT-both-from-the-start lesson learned in W1 and carried forward in W2), written best-effort after the primary fact write succeeds (an edge-write hiccup must never undo the fact itself), and exposed to the planner as a flat edge list under its own top-level `graph_overview` key, gated by the same flag so the key is absent entirely when off (not an empty array) — matching `asset_reachability`'s exact degrade behavior.
- `record_fact` has no evidence-binding mechanism at all (no `traffic_refs`/`evidence_hint_id` — that is a `report_finding`-only concept). The evidence gate for `supersedes` must instead use what `record_fact` already has: its own `confidence` field (`observed`/`inferred`) and its one-line `evidence` string.
- The planner-prompt addition must follow the established style for a gated, optional signal (`agent/planner.go`'s existing `asset_reachability` bullet): a "(when visible)" lead-in, a plain explanation of what the field means, what action to consider, an explicit statement of what it does *not* do, and a "do nothing extra when absent/empty" close.

## Review Focus

- **A `supersedes` claim with no real justification** (new fact has `confidence=inferred`, or an empty `evidence` string) must be hard-rejected, not silently accepted or silently dropped — mirrors `reaches`' "rejected outright, not silently dropped" rule.
- **A `supersedes` target that isn't an existing fact of this task** (a finding, an intent, a goal, or a fact from an unrelated task) must be rejected with a clear error, mirroring `add_intent`'s `parent_ids` validation (which already rejects non-fact/finding parents today).
- **The feature-flag default is off**, and when off, `record_fact` must reject a `supersedes` claim outright (consistent with `reaches`' own `reachabilityEdgesEnabled()` gate) rather than silently ignoring the parameter — a worker should get a clear signal to ask an operator to enable it, not a quiet no-op.
- **`graph_overview`'s new key must be entirely absent (not `[]`) when the flag is off**, matching `asset_reachability`'s degrade behavior exactly, so an unmodified planner prompt from before this feature shipped sees no difference.
- **A fact can be recorded in the same call it supersedes another** (self-reference should be rejected, mirroring `reaches`' no-self-loop rule for asset edges) — same `src != dst` principle, applied here as "a fact cannot supersede itself."

---

## Decided

- **New relation constant**: `db.RelSupersedes = "supersedes"`, added to the existing `Rel*` block in `db/constants.go`.
- **Schema migration required** (corrected from an earlier draft): `db/schema.sql`'s `ck_edge_rel` CHECK constraint must be extended to include `'supersedes'`. Unlike the existing `DO $$ ... find an auto-named constraint by pattern ... END $$` block right after the table (written once to handle pre-existing installs whose constraint predated being named `ck_edge_rel`), every install that has already run current `schema.sql` has the constraint reliably named `ck_edge_rel` — so this addition does not need that pattern-search dance. A plain, idempotent (schema.sql re-applies on every startup) update suffices:
  ```sql
  ALTER TABLE exploration_edges DROP CONSTRAINT IF EXISTS ck_edge_rel;
  ALTER TABLE exploration_edges ADD CONSTRAINT ck_edge_rel
      CHECK (rel IN ('spawns','derived_from','yields','proves','covers','supersedes'));
  ```
  Placed immediately after the existing `DO $$` block in `db/schema.sql` (~line 273).
- **No new query method.** `graphOverviewData()` (`agent/tools.go`) already loads every exploration edge once per call (`edges, _ := t.ts.Edges(5000)`, line 400) and loops it to build `parentsOf`/`yieldsOf`/`factFrom`. Reuse that same already-fetched `edges` slice with a **second** pass, run right after the existing loop (so `factFrom` is fully populated first): filter for `e.Rel == db.RelSupersedes` and look up each old fact's owning intent via the now-complete `factFrom` map. This needs no new DB round-trip and no new `ExplorationStore` method — see the exact insertion point below.
- **`record_fact` schema addition** (`agent/tools.go`'s `factItem` struct and JSON schema): a new per-item optional field, `supersedes` (array of fact node ids) — *"optional: ids of earlier facts this new fact invalidates or overturns (e.g. 'fact #1948 said this endpoint has an authz gate; this fact shows the 403 was actually a wrong-credential-type artifact'). Requires confidence=observed and a non-empty evidence on this same fact — overturning a settled conclusion needs real proof, not a guess."*
- **Gating, in `recordOneFact` (or the batch wrapper, whichever already validates per-item fields today)**, in this order, mirroring `reaches`' exact fail-closed shape:
  1. Feature flag off + `supersedes` non-empty → hard error naming the setting, e.g. *"fact supersession is disabled; omit supersedes, or ask an operator to enable fact_supersedes_enabled."*
  2. `confidence != "observed"` or `evidence == ""` on this same fact → hard error: *"supersedes requires confidence=observed and a non-empty evidence on this same fact — a claim to overturn an earlier conclusion needs real proof, not a guess."*
  3. Each id in `supersedes` must resolve to an existing `KindFact` node **of this same exploration** → hard error naming the bad id. **Scope ruling (resolved during builder-check, not left open):** the design's prose said "this task" throughout, but the builder-check review noted `add_intent`'s `parent_ids` validation — the pattern this doc says to mirror — actually uses `GetNodeWithSources` and accepts a fact from a directly-linked source task (`inherited=true`), a looser rule than the doc's own words implied. Decision: `supersedes` follows `record_fact`'s own existing `intent_id` check in the *same function* (plain `GetNode`, this-exploration-only), not `parent_ids`' cross-task allowance. Reasoning: internal consistency within one tool call outweighs consistency with a different tool's parameter serving a different purpose — `parent_ids` exists to let a hub task's new intent trace its provenance to a spoke task's fact (deliberately cross-task), whereas a negative-conclusion-was-wrong claim is inherently about re-examining this exploration's own prior work. Cost if wrong: a hub task cannot supersede a linked spoke task's stale fact (or vice versa) in v1 — a small, additive relaxation later (swapping `GetNode` for `GetNodeWithSources`) if that turns out to matter, not a breaking change.
  4. Self-reference (an id in `supersedes` equal to the fact being created) → rejected — not applicable in practice since the new fact has no id yet at validation time, but if the same id appears twice in `supersedes`, dedupe rather than erroring (harmless).
- **Edge write**: after the new fact's node is successfully written, for each validated `supersedes` target, `_ = t.ts.Link(newFactID, db.RelSupersedes, oldFactID)` — best-effort, errors swallowed, matching `reaches`' own rationale ("the fact itself is already recorded and must not be undone by an edge-write hiccup").
- **Feature flag**: new settings key `fact_supersedes_enabled` (default off), following the exact `burn_signal_enabled`/`reachability_edges_enabled` pattern: a new tiny file `agent/supersedes.go` (package var `FactSupersedesEnabled func() bool` + unexported helper, mirroring `agent/reachability.go`'s 2-line shape exactly), wired once in `server/assembly.go`'s `wireTools`, backed by `Manager.FactSupersedesEnabled()/SetFactSupersedesEnabled()` in `server/manager.go`, and present in both the `GET /api/settings` payload and the `PATCH /api/settings` handler in `server/server.go` from the start (not deferred to a final-review fix, per the W1 lesson already learned).
- **`graph_overview` exposure — corrected insertion point (resolved during builder-check):** an earlier draft placed this "immediately after the existing `asset_reachability` block" (`agent/tools.go` line ~394) and had the planner find the stale intent by searching the *truncated* `recent_done_intents` window for a matching `yields` entry. The builder-check review caught that this would silently fail for exactly the motivating scenario (a negative fact from long enough ago that its producing intent has already scrolled out of that window, or been folded into a cold digest). Fix: compute `fact_supersedes` **after** the `factFrom` map is fully built (after the loop at lines 404-412, which already reads `edges, _ := t.ts.Edges(5000)` once for `parentsOf`/`yieldsOf`/`factFrom`), and embed `old_intent_id` directly in each entry so the signal is self-contained — correct regardless of whether the old intent is still in any windowed list:
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
  This reuses the `edges` slice already in memory — no new DB call, no new `ExplorationStore` method.
- **Planner prompt bullet** (`agent/planner.go`'s `plannerDefaultTmpl`), inserted after the existing `asset_reachability` bullet, following its exact style — updated to reference `old_intent_id` directly rather than a window lookup:
  > "**Treat a superseded fact as a revival signal (when visible)**: when an admin has turned the setting on, fact_supersedes above lists entries {new_fact_id, old_fact_id, old_intent_id} — a later fact that has overturned an earlier one's conclusion, and (when present) the intent that earlier fact came from. If old_intent_id is done/blocked/exhausted because of that now-overturned conclusion and the direction is still in scope and worth pursuing, dispatch a new intent with add_intent(parent_ids=[new_fact_id], ...) explaining what changed — look up old_intent_id with node_detail if it is not in open/running/recent_done_intents already. This does not reopen the old intent or create a new goal on its own — it only flags a direction worth a fresh look. Do nothing extra when fact_supersedes is absent or empty."
  (No backticks in the inserted text — `plannerDefaultTmpl` is itself a backtick-delimited Go raw string literal; a literal backtick breaks the build, a mistake already made and fixed once this session in W1.)

## What's left to the builder

- The exact Go error-message wording for each validation failure beyond what's quoted above (the Decided section fixes the required content, not every word).
- Whether `recordOneFact`'s existing per-item validation loop is the right place to add the `supersedes` checks, or whether the batch wrapper needs a small restructure — the builder should read `agent/tools.go`'s current `recordFact()`/`recordOneFact()` functions in full before deciding (confirmed to exist at `agent/tools.go:1371-1416` as of this research, but exact line numbers will have shifted after the `reaches`-analogous additions).
