# Enhancement Phase 2 — exotic-class skills pack

An additive pack that gives the `worker`/`pentest` agents methodology for five
high-value bug classes ARTEX did not cover. Built only from skills and a REST
seed — no changes to the harness, core Go, or DB schema.

Design: `docs/superpowers/specs/2026-10-11-capability-enhancement-umbrella-design.md`
(§Phase 2). Precedent: Phase 1 (`docs/enhance-phase1.md`).

## What it adds

Five skill-gated `SKILL.md` files (no MCP — pure methodology), each carrying the
same three clauses the umbrella requires: a **depth floor** (≥25 distinct
attempts per class), an **evidence gate** (reproducible PoC/traffic before
`report_finding`), and an explicit **stop-authority disclaimer** (the class is
never declared exhausted by the skill — that stays with `hunting-methodology`).

- **`hunt-ssrf`** — in-band discovery, cloud IMDS (AWS/GCP/Azure/Alibaba),
  protocol smuggling (`gopher`/`dict`/`file`), DNS rebinding, blind
  confirmation, and the filter-bypass ladder (encoding, parser differentials,
  302, IPv6).
- **`hunt-cache-poison`** — unkeyed-header/param detection, host/port/scheme
  poisoning, parameter cloaking, Web Cache Deception, and the
  cache-busting→clean-replay proof.
- **`hunt-race-conditions`** — parallel identical requests, single-packet /
  last-byte sync, idempotency-key replay, state-transition and
  refund/cancel races, with a sequential baseline for proof.
- **`hunt-graphql-authz`** — introspection recovery, alias batching, fragment
  abuse, nested-resolver IDOR, field-level authz, and admin-mutation probes.
- **`hunt-smuggling`** — CL.TE, TE.CL, H2.CL, H2.TE, header obfuscation, and the
  time-delay / differential-response detection techniques.

The skills bind to both `worker` and `pentest`, **merged** with their existing
skills (nothing removed).

Two skills ship a lazy-loaded **reference file** (kept out of the loaded prompt
unless its condition applies):

- `hunt-ssrf/references/source-code-review.md` — grep + Semgrep patterns for when
  you have repo access.
- `hunt-graphql-authz/references/clairvoyance.md` — introspection recovery via
  the field-suggestion oracle when introspection is disabled.

## One-command setup

```bash
ARTEX_ADMIN_PASSWORD='<admin pw>' ARTEX_URL='http://localhost:8787' \
  scripts/setup-enhance-phase2.sh
```

The login user is **`Penred`** (post-rebrand). The seed is **idempotent**:
re-running re-merges the same five skills and never duplicates a binding. With no
admin password it fails fast and makes no change.

## Manual UI equivalent

**System → Agents → worker → visibility**: tick `hunt-ssrf`,
`hunt-cache-poison`, `hunt-race-conditions`, `hunt-graphql-authz`,
`hunt-smuggling`. Keep the existing skills. Repeat for `pentest`.

## Storage model

None added. Skills are files; bindings use existing agent-config columns. Zero
schema change.

## Validation

Automated checks (all green):

- Skill lint over the five `SKILL.md`:
  `for s in hunt-ssrf hunt-cache-poison hunt-race-conditions hunt-graphql-authz hunt-smuggling; do python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py skills/$s/SKILL.md; done`
- Seed script: `bash scripts/tests/test_setup_enhance_phase2.sh` — auth-guard
  (no password ⇒ no PUT), full-set binding on `worker` **and** `pentest`,
  prior skill/mcp preserved, each new skill present exactly once (dedup), and a
  second run producing an identical PUT body (idempotency), against a **stateful**
  recording stub.
- Reference integrity: `bash scripts/tests/test_phase2_skill_refs.sh` — every
  `references/<file>.md` a skill points at must exist.

**Acceptance gate (operator-run, pending):** with a running ARTEX + a self-hosted
lab —

1. Run the seed → the five skills bound to `worker` and `pentest`, prior
   bindings intact.
2. Launch a task against an SSRF lab → the worker invokes `hunt-ssrf`, walks the
   bypass ladder, and captures in-band + blind evidence.
3. Launch a task against a race lab → `hunt-race-conditions` produces the
   concurrent burst plus a sequential baseline as evidence.

Record the outcome here when run.

## Phase-2 scope note

The umbrella §Phase 2 lists ~14 exotic classes plus a recon pack. This pack
delivers the five highest-value classes; prototype pollution, mass assignment,
deserialization, SSTI, XXE, CORS, and subdomain takeover are **deferred** to a
later pack.
