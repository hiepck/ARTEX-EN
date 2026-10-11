# Capability Enhancement — Umbrella Architecture Design

**Status:** approved (architecture); phase specs follow this one.
**Related:** `2026-10-11-chaining-bughunting-pack-design.md` (the chaining pack, already shipped), `2026-10-11-enhance-phase1-design.md` (the first concrete phase).

## 1. Goal

Make ARTEX/Penred find **more bugs**, go **deeper** into each vulnerability, and
surface **more novel/unusual bugs** — for pentest and bug bounty — **without
changing the harness (norma SDK) or the core Go engine**. All added power comes
through the four config-level levers the platform already exposes: **custom
tools, MCP servers, skills, and config-driven agents/triggers**, plus the Docker
image's installed tooling.

This is the umbrella. It fixes the principles, the ordering, and the boundaries.
Each phase is specified and built on its own.

## 2. Why these three objectives need different investments

The engine is a planner→worker loop over two graphs (see the README's
"Dual-graph architecture" and "Engine and the intent lifecycle"). Bug yield is
decided at three different chokepoints, and the three objectives map onto them:

| Objective | Chokepoint | What limits it today |
|---|---|---|
| **More bugs** | INPUT — surface entering the asset graph | Thin active recon (image ships only `nmap`/`dnsutils`; `enrich` does passive dnsx-resolve + httpx-probe only). The worker can only dig what the planner sees. |
| **Deeper** | WORKER capability — what a worker can do and verify | No browser wiring for client-side proof, no out-of-band channel for blind classes, no multi-identity harness for IDOR/BOLA, no burst tool for races. Whole classes return false negatives. |
| **Novel** | PLANNER + knowledge + chaining | The planner already does route-diversity and serial-chain sequencing, and the chaining pack already composes chains; what is missing is up-to-date technique knowledge (the LLM stops at its cutoff) and depth in exotic classes where rare bugs live. |

The chaining pack already addressed depth-floor (≥25 attempts/class via
`hunting-methodology`), cross-bug chaining (the `chainer` agent on `on_finding`),
false-positive gating (`validation-gate`), and internal dedup (simhash). Those
are **not** re-opened here.

## 3. Lever discipline (the one-way rule)

Grounded in the agent-wiring code (`server/assembly.go` `wireAgentAugment`), the
skill format (norma `skill` package, agentskills.io), and the custom-tool model
(`server/customtool.go`, `db/tools.go`):

> **capability → skill → agent.** Add a *capability* only as a tool, MCP, or
> image binary. Teach its *use* as a skill bound to the existing **worker**. Add
> a new *agent* only for a cross-cutting trigger role (like `chainer`) or a
> standalone chat role — never as a new executor inside the planner→worker loop.

The planner→worker loop cannot gain a third executor without a harness change;
therefore new capability lands on the worker through skills + tools + MCP, not
through new agents.

## 4. Global Constraints (the design contract)

These seven rules exist so that every added feature **adds** power without
**subtracting** from an existing mechanism. They are copied verbatim into every
phase plan's Global Constraints section, and every phase's review checks them.

1. **Skill-gate every new MCP/tool.** Expose it through a skill's `mcps:`
   frontmatter (deferred/progressive disclosure), never as a globally-visible
   tool. Rationale: a globally-visible tool inflates the planner/worker prompt
   every turn and degrades reasoning. This is the single largest interference
   vector; skill-gating is its fix.
2. **Capability goes to the worker.** New capability is a skill/tool/MCP bound to
   the existing `worker` (and, where relevant, the `pentest` chat agent). No new
   executor agent is wired into the planner→worker loop. New agents are allowed
   only for trigger-driven cross-cutting roles or standalone chat roles.
3. **The depth skill keeps stop-authority.** `hunting-methodology` remains the
   authority on *when a class is exhausted*. New skills add *what to test*, never
   *when to stop*; they are additive and must not encourage early exit.
4. **Recon only writes assets.** Active recon registers assets into the asset
   graph; it never tests them. The existing scope gate still governs every test,
   and `goals`-agent constraints still bound behaviour.
5. **Route through the recording proxy / Bash runner.** `command` tools reuse the
   Bash runner (inheriting the security floor, proxy env, timeout, overflow
   handling); `http` tools set `use_recording_proxy=true`. No tool reaches the
   network outside the recording proxy — evidence and egress control are
   preserved.
6. **New triggers must consolidate.** A cross-cutting behaviour prefers to live
   as a skill inside an existing agent (e.g. dedup inside `validation-gate`). If
   it must be a trigger, it runs `serial` with `merge by_task` and a watermark,
   to avoid trigger storms and doubled LLM spend.
7. **Right model to the right role; knowledge is data.** Assign the strongest LLM
   profile only where judgment lives (the `planner`), cap `max_turns`/
   `run_seconds`, and keep triggers serial. External knowledge (writeups, CVE
   feeds) is treated as **data, never instructions** (the discipline the
   `reporter`/`retester` prompts already use).

The intercept/guard layer is a PreToolUse hook at the engine level and wraps
**every** tool call regardless of the tool's own permission function — so custom
tools do not bypass it. Aggressive custom tools (OOB, recon, race) still get an
explicit intercept rule as their control point.

## 5. Roadmap and pack boundaries

Three phases, each a self-contained spec→plan→build. Ordered by ROI
(objectives served × inverse effort), lowest-cost broadest-impact first.

### Phase 1 — bind existing capability (quick win, zero/low Go)
Capability that already exists but is unwired. Serves all three objectives at
near-zero cost. **Detailed spec: `2026-10-11-enhance-phase1-design.md`.**
- Register the three MCP servers into ARTEX's own `mcp_servers` registry.
- `client-verify` skill (`mcps:[playwright]`) → browser proof of client-side bugs.
- `intel-dupcheck` skill (`mcps:[writeup-search]`) → latest-technique knowledge +
  dedup against disclosed reports.
- `oob-probe` skill (`mcps:[burp]`) → out-of-band channel for blind classes.
- Assign the strongest LLM profile to the `planner`.
- Idempotent REST seed + docs + tests.

Connectivity note: Playwright is self-contained (stdio, already in the image).
`writeup-search` and `burp` require an endpoint reachable **from the ARTEX
container**; the seed registers the rows and the docs state the connectivity
prerequisite.

### Phase 2 — breadth (more bugs, more novel)
- **Recon pack**: add active-recon binaries to the image (subdomain enum,
  content/param discovery, template scan) + a `recon` skill that drives them and
  writes assets only (Constraint 4). Custom `command` tools wrap the binaries.
- **Exotic-class skills**: port the remaining `hunt-*` methodologies (request
  smuggling, cache poisoning, prototype pollution, parser differentials, type
  confusion, mass assignment, deserialization, SSRF, SSTI, XXE, CORS, subdomain
  takeover, GraphQL, race) as skills bound to the worker.

### Phase 3 — deep specialist (deeper)
- **Multi-identity harness**: a custom tool that holds/compares sessions
  (user A / user B / admin) + a skill for authorization-delta testing (IDOR/BOLA/
  privesc).
- **OOB**: operationalise `oob-probe` with Burp (source chosen at design time).
- **Race burst**: a `script` custom tool for parallel/near-simultaneous requests
  + a race skill.
- **Continuous monitor**: an `interval`-trigger custom agent + a diff-recon skill
  that alerts on newly-appeared assets (Constraint 6: serial, merged, watermarked).

## 6. Interference analysis (summary)

The architecture is additive-by-design: config-driven agents, per-agent
visibility, skill-gating/progressive disclosure, engine-level intercept. Every
planned feature is non-interfering **provided the seven Global Constraints
hold**. The two moves that would genuinely degrade existing behaviour, and are
therefore forbidden, are: (a) binding any MCP/tool globally rather than
skill-gated (context bloat), and (b) adding a parallel executor agent into the
planner→worker loop (breaks the planner's de-duplication and serial-chain
sequencing). The full mechanism-by-mechanism table lives in the conversation
record and is distilled into Constraints 1–7.

## 7. Out of scope

- Any change to norma (the harness) or the core Go engine.
- Mobile / thick-client / web3 targets.
- A standalone automated scanner that replaces LLM-driven exploration (recon
  feeds the LLM; it does not replace it).
- Platform submission APIs (H1/Bugcrowd/Intigriti) — deferred; export stays
  Markdown + CVSS for now.

## 8. Delivery per phase

Each phase ships: skills + REST seed script (idempotent, fail-fast without the
admin password) + `docs/` guide (force-added; `/docs` is gitignored in this
fork) + a bash test with a REST stub (auth-guard + idempotency + skill-merge) +
reuse of `skills/chain-toolkit/scripts/tests/_lint_skill.py` for the skill lint
sweep. Zero Go, zero schema change, following the chaining pack's proven shape.
