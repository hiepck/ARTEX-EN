# Chaining + Bug-Hunting Pack

An additive pack that deepens ARTEX's bug finding and chains discovered bugs into
higher-impact composite findings. It is built only from skills, a custom agent, tool
bindings, and shipped Python scripts — no changes to the harness, core Go, or DB schema.
Design spec: `docs/superpowers/specs/2026-10-11-chaining-bughunting-pack-design.md`.

## What it adds

- **Skills** under `skills/`: `hunting-methodology`, `chain-hunting` (+ `references/chain-table.md`,
  `references/payloads.md`), `validation-gate` (+ `references/never-submit.md`), the per-class
  `hunt-idor` / `hunt-oauth` / `hunt-xss` / `hunt-rce` / `hunt-business-logic` / `hunt-info-disclosure`,
  and `chain-toolkit` (the Python helpers).
- A **`chainer`** custom agent that fires on every new finding (the `on_finding` trigger),
  correlates findings, ranks next-hops, and plants a chain-pursuit hint for the planner
  to dispatch. It never drives target traffic itself.
- A **`chain-toolkit`** of stateless Python scripts (`chain_plan.py`, `cvss_calc.py`,
  `dedup.py`, `simhash.py`) called via `Bash`.

## Prerequisites

- A running ARTEX with its admin password set (first-run `/setup`).
- `curl`, `jq`, and `python3` available where you run the seed script and where the
  agent runs (`python3` for the toolkit).

## One-command setup

```bash
ARTEX_ADMIN_PASSWORD='<your admin password>' \
ARTEX_URL='http://127.0.0.1:8787' \
scripts/setup-chaining-pack.sh
```

The script logs in, creates the `chainer` agent (if absent), sets its prompt, binds the
`chain-hunting` / `chain-toolkit` / `validation-gate` skills, binds the chainer's tools,
creates an `on_finding` trigger, and **also binds the hunting skills (`hunting-methodology`,
the `hunt-*` set, `validation-gate`) to the `worker`** — merged with the worker's existing
skills, so its per-class bug finding gets deeper too. **It is idempotent** — re-running updates in place
and never creates a second agent or trigger. With no password set it fails fast and makes
no changes.

Make sure the skills directory the backend loads (`ARTEX_SKILL_DIR` > config `skill_dir` >
`skills/` next to the binary) is the one containing this pack.

## Manual UI equivalent

If you prefer the UI over the seed script:

1. **Create a custom agent** with key `chainer`, name "Chainer".
2. **Paste the prompt** (same text the seed script sets): the agent correlates findings,
   ranks next-hops, plants `add_task_hint` chain-pursuit hints for in-scope hops, enforces
   the per-hop evidence gate, re-checks scope every hop, and composes the composite with
   `report_finding` at a terminal impact. It does not drive target traffic.
3. **Bind skills** (agent visibility): `chain-hunting`, `chain-toolkit`, `validation-gate`.
4. **Bind tools** to the agent: `list_findings`, `list_task_findings`, `get_task_node_detail`,
   `get_task_graph`, `list_task_worker_traces`, `get_task_worker_trace`, `add_task_hint`,
   `Bash`, and `report_finding` (the last for the composite only). Do **not** bind
   `record_fact`: it requires a task-bound exploration store and always errors with
   "not running inside a task" for a cross-task triggered agent — `add_task_hint` (which
   takes `task_id` explicitly) is the correct way for the chainer to write into a task's graph.
5. **Add a trigger**: condition `on_finding`, enabled, with a finding message telling the
   agent to correlate the new finding and plant a chain-pursuit hint for in-scope hops.

6. **Bind the hunting skills to the worker** (agent visibility): `hunting-methodology`, the
   `hunt-*` set, and `validation-gate` — so the worker's per-class bug finding gets deeper.
   Keep the worker's existing skills; add these to them.

## Storage model (composite findings)

A chain is stored as a **composite row in the existing `findings` table** — no schema
change — created with the normal `report_finding` tool:

| Field | Value |
|---|---|
| `vulnclass` | `chain` |
| `name` | the terminal impact, e.g. `SSRF → IMDS → account takeover` |
| `severity` | the bucket from `cvss_calc.py` scored on the terminal impact |
| `summary` | one-line chain statement (legs → impact) |
| `evidence` | per-hop proof pointers **and the ordered leg finding IDs** |
| `report` | the full Markdown chain narrative (written by the reporter workflow) |
| `traffic_refs` | the combined, ordered traffic evidence across the legs |

The individual leg findings remain their own `findings` rows and live in the same
exploration graph, so the chain is navigable there; the composite references the legs by
id. The reporter agent writes the composite's Markdown report through the existing finding
workflow.

## Evidence gate and scope rule

- **Per hop:** the postcondition must be observed on a second, independent action (a leaked
  credential must authenticate a fresh request; a 200 alone is not proof). The hop's
  precondition must point at a proven postcondition of an earlier hop.
- **Chain confidence = min across hops.** Any single theoretical (un-triggered) hop makes
  the whole chain non-reportable as a chain.
- **Scope is re-checked on every hop** via the worker's existing scope gate. An out-of-scope
  hop is a hard stop — the chain is marked blocked and not pursued.

See the `chain-hunting` and `validation-gate` skills for the full methodology.

## Validation run

Automated checks (run on 2026-10-11, all green):

- Python toolkit suite: `chain_plan`, `dedup`, `cvss_calc`, `_lint_skill` — all pass
  (`python3 skills/chain-toolkit/scripts/tests/<name>.py`).
- Seed script: `bash scripts/tests/test_setup_chaining_pack.sh` — pass (auth-guard +
  idempotency against a recording stub).
- Lint sweep over all 43 pack markdown files — clean.

**Acceptance gate — run on 2026-10-24 against a live authorized engagement (task #597,
real target, not a self-hosted lab):** the pack was activated with the seed script's exact
API sequence, then observed live. A worker-reported SSRF finding (unauthenticated
Collabora `upload-settings` request-injection reaching an internal admin host) fired the
`on_finding` trigger; `chainer` correlated it, independently researched both candidate
next hops against the real open-source code of the reached services, and planted two
`add_task_hint` hints: (1) a precise, evidence-gated next-hop probe for a genuine
debug-mode local-file-read primitive it located in the reachable service's source, with an
explicit two-independent-reads evidence bar before escalating further, and (2) an explicit
"chain BLOCKED, do not report a theoretical chain" instruction for the other candidate hop
once it determined the hop was not actually reachable — i.e. the pack suppressed a
would-be false-positive chain rather than reporting it. It did not register a composite
finding, correctly, since no hop had yet reached a proven terminal impact (the
min-across-hops evidence gate held). One defect found and fixed during this run:
`record_fact` was bound to the chainer per this doc's earlier tool list but can never
succeed for a cross-task triggered agent (see "Manual UI equivalent" above) — removed from
both the seed script and this doc. No other defects observed.
