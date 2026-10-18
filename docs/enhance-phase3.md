# Enhancement Phase 3 — exotic-class skills pack (deferred-7)

An additive pack that gives the `worker`/`pentest` agents methodology for the
seven high-value bug classes Phase 2 explicitly deferred. Built only from
skills and a REST seed — no changes to the harness, core Go, or DB schema.

Design: `docs/superpowers/specs/2026-10-11-capability-enhancement-umbrella-design.md`
(§Phase 2 / deferred classes). Precedent: Phase 1 (`docs/enhance-phase1.md`),
Phase 2 (`docs/enhance-phase2.md`).

## What it adds

Seven skill-gated `SKILL.md` files (no MCP — pure methodology), each carrying
the same three clauses the umbrella requires: a **depth floor** (≥25 distinct
attempts per class), an **evidence gate** (reproducible PoC/traffic before
`report_finding`), and an explicit **stop-authority disclaimer** (the class is
never declared exhausted by the skill — that stays with `hunting-methodology`).

- **`hunt-prototype-pollution`** — client-side and server-side (SSPP)
  `__proto__`/`constructor.prototype` key injection via JSON body merges,
  query-string-to-object parsing (`qs`, `$.extend`, lodash `merge`/
  `defaultsDeep`), or deep-clone utilities; covers RCE chains (`child_process`,
  EJS/Pug/Handlebars) and DOM XSS chains.
- **`hunt-mass-assignment`** — over-posting extra JSON fields (`role`,
  `isAdmin`, `price`, `balance`, `verified`, `userId`, `discountPercent`) that
  a backend binds wholesale to a model/struct with no allowlist; covers Rails
  strong-params gaps, Django `ModelForm` over-exposure, ASP.NET and Spring
  model binding, and ORM auto-serialization (Mongoose, Sequelize, TypeORM).
- **`hunt-deserialization`** — insecure deserialization across Java
  (`ObjectInputStream`, ysoserial gadget chains), PHP (`unserialize()` +
  `__wakeup`/`__destruct`/`__toString` abuse), Python (`pickle.loads`, unsafe
  `yaml.load`), .NET (`BinaryFormatter`), and Ruby (`Marshal.load`,
  `YAML.load`/`unsafe_load`).
- **`hunt-ssti`** — Server-Side Template Injection across Jinja2/Twig,
  FreeMarker/Velocity/Thymeleaf (Java), and ERB/Handlebars (Ruby/Node);
  covers polyglot engine fingerprinting, the parse-error-vs-runtime-evaluation
  diagnostic, and sandbox-escape via object introspection.
- **`hunt-xxe`** — XML External Entity injection: classic in-band file-read
  via `<!ENTITY>`, blind/OOB XXE via parameter entities and an external DTD,
  SOAP services, and upload-based XXE in SVG/DOCX/XLSX.
- **`hunt-cors`** — CORS misconfiguration: origin reflection combined with
  `Access-Control-Allow-Credentials: true`, null-origin bypass,
  allowlist-regex bypasses, and simple-request tricks that skip preflight.
- **`hunt-subdomain-takeover`** — dangling CNAME records pointing at
  unclaimed cloud resources (S3, Azure, GitHub Pages, Heroku, Fastly,
  Shopify, WordPress.com) and dangling NS delegation; fingerprint matching
  against each service's unclaimed-error signature, defaulting to reporting
  evidence over claiming.

The skills bind to both `worker` and `pentest`, **merged** with their existing
skills (nothing removed — Phase-1 and Phase-2 bindings are untouched).

No skill in this pack ships a lazy-loaded `references/` sub-file — all seven
fit comfortably in a single `SKILL.md` each (verified: see Validation below).

## One-command setup

```bash
ARTEX_ADMIN_PASSWORD='<admin pw>' ARTEX_URL='http://localhost:8787' \
  scripts/setup-enhance-phase3.sh
```

The login user is **`Penred`** (post-rebrand). The seed is **idempotent**:
re-running re-merges the same seven skills and never duplicates a binding.
With no admin password it fails fast and makes no change.

## Manual UI equivalent

**System → Agents → worker → visibility**: tick `hunt-prototype-pollution`,
`hunt-mass-assignment`, `hunt-deserialization`, `hunt-ssti`, `hunt-xxe`,
`hunt-cors`, `hunt-subdomain-takeover`. Keep the existing skills. Repeat for
`pentest`.

## Storage model

None added. Skills are files; bindings use existing agent-config columns. Zero
schema change.

## Validation

Automated checks (all green):

- Skill lint over the seven `SKILL.md`:
  `for s in hunt-prototype-pollution hunt-mass-assignment hunt-deserialization hunt-ssti hunt-xxe hunt-cors hunt-subdomain-takeover; do python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py skills/$s/SKILL.md; done`
  — no `LINT FAIL`.
- Seed script: `bash scripts/tests/test_setup_enhance_phase3.sh` — auth-guard
  (no password ⇒ no PUT), full-set binding on `worker` **and** `pentest`,
  prior skill/mcp preserved, each new skill present exactly once (dedup), and
  a second run producing an identical PUT body (idempotency), against a
  **stateful** recording stub. `ALL TESTS PASSED`.
- Reference integrity: `bash scripts/tests/test_phase3_skill_refs.sh` — every
  `references/<file>.md` a skill points at must exist (none of the seven
  reference one). `ALL REFS PRESENT`.

**Acceptance gate (operator-run, pending):** with a running ARTEX + a
self-hosted lab —

1. Run the seed → the seven skills bound to `worker` and `pentest`, prior
   bindings (Phase 1 + Phase 2) intact.
2. Launch a task against an SSTI lab → the worker invokes `hunt-ssti`,
   fingerprints the engine via the polyglot probe, and escalates to
   evaluation or sandbox-escape evidence.
3. Launch a task against an XXE lab → `hunt-xxe` produces in-band or OOB
   file-read evidence from a crafted DTD/entity payload.
4. Launch a task against a target with a dangling CNAME →
   `hunt-subdomain-takeover` fingerprints the unclaimed resource and defaults
   to reporting evidence rather than claiming it.

This pack has **not** been run against a live vulnerable target. Unlike
Phase 2 (whose item 1 — seed → binding, discovery — was verified against a
live instance; see `docs/skill-binding-verification.md`), no live-instance
verification pass has been performed for Phase 3 as of this writing. The
automated checks above confirm the skills parse, lint, and bind correctly in
a stubbed environment; they do not confirm real-world hunting efficacy.
Record the acceptance-gate outcome here when it is run.

## Phase-3 scope note

This pack closes out the Phase-2 umbrella's deferred list: all seven classes
named in `docs/enhance-phase2.md`'s former scope note (prototype pollution,
mass assignment, deserialization, SSTI, XXE, CORS, subdomain takeover) now
ship here. The umbrella's §Phase 2 recon pack (beyond these seven hunting
classes) remains out of scope for this pack.
