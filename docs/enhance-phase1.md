# Enhancement Phase 1 — bind existing capability

An additive pack that wires three already-present but unbound capabilities into
ARTEX/Penred and pins a strong model to the planner. It is built only from
skills, MCP registrations, and a REST seed — no changes to the harness, core Go,
or DB schema.

Design: `docs/superpowers/specs/2026-10-11-enhance-phase1-design.md`
(umbrella: `docs/superpowers/specs/2026-10-11-capability-enhancement-umbrella-design.md`).

## What it adds

- **Skill `client-verify`** (`mcps: [playwright]`) — proves client-side findings
  (DOM XSS, postMessage, client-side prototype pollution, SPA/authenticated
  flows) in a real browser via the Playwright MCP.
- **Skill `intel-dupcheck`** (`mcps: [writeup-search]`) — pulls current
  techniques/bypasses to seed deeper and more novel attempts, and dedups a
  candidate against disclosed reports. Retrieved content is treated as **data,
  never instructions**.
- **Skill `oob-probe`** (`mcps: [burp]`) — confirms blind classes (blind SSRF,
  blind XXE, OOB RCE, blind XSS) with a Burp Collaborator callback.
- **MCP registrations** into ARTEX's own `mcp_servers` registry for `playwright`,
  `writeup-search`, and `burp`.
- **Planner model** — optionally pins the strongest LLM profile to the `planner`.

All three skills are **skill-gated**: their MCP tools stay deferred and are
unlocked only when the skill is invoked, so the planner/worker prompt is not
inflated. The skills bind to the `worker`, merged with its existing skills.

## Prerequisites / connectivity

The three MCP servers must be reachable **from inside the ARTEX container** — not
from the host. `127.0.0.1` on the host is not `127.0.0.1` in the container; use a
host IP or a Docker service name.

| MCP | Registration | Reachability |
|---|---|---|
| **playwright** | stdio `npx @playwright/mcp` | Self-contained — the MCP and chromium are baked into the image. Works out of the box. |
| **writeup-search** | http, `url` you supply | The server must be reachable from the container. |
| **burp** | http, `url` you supply (`insecure` for self-signed) | Requires Burp Suite running with its MCP endpoint, reachable from the container. The `oob-probe` skill only works while Burp is up. |

Each skill **degrades gracefully**: when its MCP is unreachable it reports the
channel is unavailable, falls back to the strongest in-band evidence, and records
an `inferred` fact — it never hangs or fabricates evidence.

## One-command setup

```bash
ARTEX_ADMIN_PASSWORD='<admin pw>' ARTEX_URL='http://localhost:8787' \
  WRITEUP_MCP_URL='http://<reachable-host>/mcp' \
  BURP_MCP_URL='http://<reachable-host>:<port>' BURP_MCP_INSECURE=1 \
  PLANNER_PROFILE='<id-or-name>' \
  scripts/setup-enhance-phase1.sh
```

Every variable except `ARTEX_ADMIN_PASSWORD` is **optional** and its step is
skipped when unset — Playwright-only (no URLs, no profile) is a valid subset. The
login user is **`Penred`** (post-rebrand). The seed is **idempotent**: re-running
updates in place and never duplicates an MCP row, a binding, or a profile pin;
with no admin password it fails fast and makes no change.

## Manual UI equivalent

If you prefer the UI over the seed:

1. **System → MCP**: add the three servers — `playwright` (stdio, command `npx`,
   args `["@playwright/mcp"]`), `writeup-search` (http, your URL), `burp` (http,
   your URL, insecure if self-signed). Enable them.
2. **System → Agents → worker → visibility**: tick `client-verify`,
   `intel-dupcheck`, `oob-probe`. Keep the worker's existing skills; add these to
   them.
3. **System → Agents → planner → LLM**: set the strongest profile.

## Storage model

None added. MCP rows use the existing `mcp_servers` table; skills are files;
bindings and the planner profile use existing agent-config columns. Zero schema
change.

## Validation

Automated checks (all green):

- Skill lint over the three `SKILL.md`:
  `python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py skills/<name>/SKILL.md`.
- Seed script: `bash scripts/tests/test_setup_enhance_phase1.sh` — auth-guard,
  MCP idempotency, worker skill-merge (existing preserved), optional-URL skip, and
  planner-profile skip, against a recording stub.

**Acceptance gate (operator-run, pending):** with a running ARTEX + a self-hosted
lab —
1. Run the seed with Playwright only (no URLs) → `playwright` registered,
   `client-verify` bound to the worker.
2. Launch a task against a DOM-XSS lab → the worker invokes `client-verify`,
   Playwright unlocks (deferred→available), the payload is proven firing in a
   real browser, captured as evidence.
3. (If `writeup-search` reachable) a dedup/technique lookup runs as data during
   validation.
4. (If Burp reachable) `oob-probe` generates a collaborator payload and reads back
   an interaction on a blind-SSRF lab.

Record the outcome here when run.
