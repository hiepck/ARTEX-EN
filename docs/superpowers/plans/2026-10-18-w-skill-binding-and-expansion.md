# W-Skill: Verify Skill Binding + Author 7 Deferred Exotic Classes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Confirm (and fix if broken) that ARTEX's own skill-loading mechanism actually discovers and can bind the `skills/hunt-*` packs already authored in this repo, audit the per-agent skill-description byte budget before adding more, then author the 7 exotic-class skills the phase-2 pack explicitly deferred (prototype pollution, mass assignment, deserialization, SSTI, XXE, CORS, subdomain takeover) in the same pattern, and wire a phase-3 setup seed + tests.

**Architecture:** Pure skill-pack work — `SKILL.md` files (agentskills.io format: YAML frontmatter + Markdown body) under `skills/<name>/`, an idempotent bash REST seed (`scripts/setup-enhance-phase3.sh`) that binds them to `worker`/`pentest` via `PUT /api/agents/{agent}/visibility`, and bash tests against a stateful Python stub server — the exact pattern phase 1/2 already established. Zero Go/DB/schema changes (confirmed precedent: `docs/enhance-phase2.md`'s "Storage model: None added").

**Tech Stack:** Markdown (SKILL.md), Bash, Python 3 (stdlib only — lint script + test stub), `curl`/`jq` for the live-instance verification task.

**Spec:** `docs/superpowers/specs/2026-10-17-core-engine-finding-capability-design.md` (workstream W-Skill section).

## Global Constraints

- **Skill format**: frontmatter has `name` + `description` only (no `sources`/`report_count`/`generated_at` — those are Claude-Code-pack conventions norma's loader silently drops; keep them out so the file is unambiguous about which system it targets). See `skills/hunt-ssrf/SKILL.md:1-3` for the minimal correct shape.
- **Three required clauses per new SKILL.md** (verbatim requirement from `docs/enhance-phase2.md`, carried forward): a **depth floor** (≥25 distinct attempts/bypass families per class before declaring it exhausted), an **evidence gate** (what must be captured before `report_finding` — request/response, traffic_refs, or equivalent proof for that class), and a **stop-authority disclaimer** (the skill decides *what* to try and *how* to verify; it never decides the class is exhausted — that stays with `hunting-methodology`). Copy `skills/hunt-ssrf/SKILL.md`'s `## Depth floor` / `## Evidence gate` / `## Stop-authority` sections verbatim in structure, adapted in content per class.
- **No schema/Go changes**: every task in this plan touches only `skills/`, `scripts/`, `scripts/tests/`, and `docs/` — if a task seems to need a Go/DB change, that is a signal that the plan or the spec is wrong, not a passing deviation.
- **Idempotent seed**: `setup-enhance-phase3.sh` must merge (never duplicate, never drop prior skills/mcp) — follow `scripts/setup-enhance-phase2.sh`'s exact `merge_visibility()` pattern.
- **Lint gate**: every new `SKILL.md` must pass `python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py skills/<name>/SKILL.md` (exit 0) before being considered done.

## Review Focus

- **A skill that looks right in isolation but silently never loads** (bound to an agent whose `skillDir` doesn't actually contain it, or a name mismatch) → Task 1 must prove discovery end-to-end against a live instance, not just assert the files exist on disk.
- **Skill-description budget overflow** (the `Skill` meta-tool caps the rendered listing at ~8000 chars total / ~500 chars per description, degrading silently to name-only past budget, per the W-Skill design doc) → Task 2 must produce an actual byte count, not just "looks fine."
- **A new skill's required clause is present in wording but wrong in substance** (e.g. a depth floor that says "25" but for the wrong unit, or an evidence gate that doesn't name what to actually capture for THAT class) → each authoring task's steps call out the class-specific evidence artifact explicitly, not a copy-pasted generic line.
- **Dangling lazy-load reference** (a skill's body points at `references/foo.md` that was never created) → Task 10's reference-integrity test (cloned from `test_phase2_skill_refs.sh`) must cover all 7 new skills, and any task that adds a `references/` file must be caught by it.
- **Non-idempotent or destructive seed** (a second run of `setup-enhance-phase3.sh` duplicates skills or drops an existing binding) → Task 10's cloned `test_setup_enhance_phase3.sh` must include the phase-2 test's exact idempotency check (run twice, compare PUT bodies byte-for-byte).

---

### Task 1: Verify skill discovery against a live instance; fix the root cause if broken

**Files:**
- Modify (only if a gap is found): whichever of `config/config.go`, `scripts/setup-enhance-phase1.sh`, `scripts/setup-enhance-phase2.sh`, or a docs file turns out to be wrong — do not pre-guess which; this task's own investigation determines it.
- Create: a short findings note appended to `docs/enhance-phase2.md` (or a new `docs/skill-binding-verification.md` if the finding is substantial enough to deserve its own doc) recording what was verified and how.

**Interfaces:**
- Consumes: `GET /api/skills` (lists skills the server's configured `skillDir` actually contains — `server/server.go:881`), `GET /api/agents/{agent}/visibility` and `PUT /api/agents/{agent}/visibility` (current/desired skill bindings), `config.SkillDir()` (`config/config.go:116`, resolution: `ARTEX_SKILL_DIR` env > `config.json` `skill_dir` > `BaseDir()/skills`).
- Produces: a pass/fail determination (recorded in the findings note) that downstream tasks and future skill packs can trust: "building and running ARTEX the standard way (`go build -tags embedui -o artex ./cmd/artex` or the no-tag stub build, from the repo root) auto-discovers `./skills/`, and `PUT .../visibility` binding-by-name against an existing directory actually works end to end." No other task in this plan depends on a specific outcome here, only on the determination existing and being correct.

- [ ] **Step 1: Stand up a live instance**

Build from the repo root (no frontend needed for this API-only check — the no-tag build ships the stub webui handler, which is fine): `CGO_ENABLED=0 go build -o /tmp/artex-verify ./cmd/artex` (use a scratch output path so this doesn't collide with any dev binary). Start a throwaway Postgres (same pattern as the W0 plan: a disposable `postgres:16` docker container) and run `/tmp/artex-verify -addr :18787` from the **repo root** (so `BaseDir()` resolves to the repo root and the default `skillDir` is this repo's `./skills/`), with `ARTEX_PG_DSN` pointing at the throwaway DB.

- [ ] **Step 2: Complete whatever first-run bootstrap the server requires**

Per `CLAUDE.md`: "First run serves `/setup` to set the admin password." Discover the exact mechanism by hitting the running instance (`curl -s http://127.0.0.1:18787/api/...` — inspect what a fresh boot exposes; the setup scripts' own convention assumes a login at `POST /api/auth/login` with `username:"Penred"`). Record whatever sequence actually works as the step's output — this is a mechanical discovery, not a design choice.

- [ ] **Step 3: Confirm skill discovery**

`curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18787/api/skills | jq -r '.[].name'` (or whatever shape the endpoint actually returns — adjust the `jq` filter to match). Expected: every directory under `./skills/` (including `hunt-ssrf`, `hunt-idor`, `chain-hunting`, `validation-gate`, etc. — the full existing set) appears in the output.

- [ ] **Step 4: Confirm binding actually works end to end**

Run `ARTEX_URL=http://127.0.0.1:18787 ARTEX_ADMIN_PASSWORD=<whatever Step 2 set> bash scripts/setup-enhance-phase2.sh`, then `curl ... http://127.0.0.1:18787/api/agents/worker/visibility` and confirm the 5 phase-2 skill names are present in the response's `skill` array.

- [ ] **Step 5: Record the determination and fix if needed**

If Steps 3-4 both succeed as expected: write the findings note (per **Files** above) stating the mechanism is confirmed working end-to-end with zero code changes needed — this closes the design doc's open question. If either step reveals a gap (a skill missing from Step 3's listing, or Step 4's binding not reflecting in a way the engine would actually use), diagnose the exact cause (wrong `skillDir` resolution, a build/deploy step that doesn't carry `skills/` alongside the binary, a naming mismatch) and apply the smallest fix that closes it, then re-run Steps 3-4 to confirm.

- [ ] **Step 6: Tear down and commit**

Stop the instance and throwaway Postgres container. Commit the findings note (and any fix from Step 5).

```bash
git add docs/
git commit -m "docs(skill): verify live skill discovery + binding end to end"
```

---

### Task 2: Audit the skill-description byte budget

**Files:**
- Create: a short findings note, either appended to the same doc Task 1 created or a new short section in `docs/enhance-phase2.md`.
- Modify (only if an existing skill's `description` is found to be excessively long): that skill's `SKILL.md` frontmatter.

**Interfaces:**
- Consumes: the live instance from Task 1 (or a fresh one, same build/run steps) and `GET /api/agents/worker/visibility` / `GET /api/agents/pentest/visibility` to get the currently-bound skill name list; `skills/<name>/SKILL.md`'s frontmatter `description` field for each bound name.
- Produces: a byte-count table (skill name → description length) and a total, that Tasks 3-9 can check their own new skill's description against before adding it, so no later task silently pushes an earlier one past the ~8000-char/~500-char-per-skill budget (per the design doc's finding on `norma/skill/skill.go`'s `maxSkillListingChars`/`maxSkillDescChars`).

- [ ] **Step 1: Enumerate currently-bound skills and their description lengths**

For each name in `worker`'s and `pentest`'s current visibility list (from Task 1's live instance, or a fresh boot of the same binary), read `skills/<name>/SKILL.md`'s frontmatter `description:` line and record its character length. A one-off shell loop is sufficient (no need for a reusable script): e.g. `for f in skills/*/SKILL.md; do awk '/^description:/{print length($0)-13, FILENAME}' "$f"; done` (adjust the offset for the `description: ` prefix length).

- [ ] **Step 2: Compute the total and compare against budget**

Sum the lengths for every currently-bound skill (not every skill in the repo — only ones actually bound to `worker`/`pentest`, since that's what the `Skill` meta-tool renders per agent). Compare against the ~8000-char total budget. Record the result (e.g. "current total: N chars, M chars of headroom before the 7 new skills").

- [ ] **Step 3: Trim any outlier found**

If any single skill's description exceeds ~500 chars (the earlier research flagged `hunt-idor`'s as the known outlier, ~1400 chars), shorten it to the essential trigger criteria only (what it's for, when to invoke) — move any narrative/CVE/report-count detail out of the frontmatter `description` and into the skill's body if it's worth keeping at all. Re-run the length check from Step 1 on the trimmed file to confirm it's now under budget.

- [ ] **Step 4: Record and commit**

Write the findings note with the table + total + headroom figure, so Tasks 3-9 know their budget. Commit.

```bash
git add skills/ docs/
git commit -m "docs(skill): audit skill-description byte budget before phase-3 additions"
```

---

### Task 3: Author `hunt-prototype-pollution`

**Files:**
- Create: `skills/hunt-prototype-pollution/SKILL.md`

**Interfaces:**
- Consumes: Task 2's headroom figure (keep this skill's `description` well under 500 chars, and check running total against Task 2's recorded budget before moving to Task 4).
- Produces: nothing other tasks directly consume by name — each of Tasks 3-9 is an independent deliverable (a reviewer can accept this one while rejecting a sibling).

This task is the fullest-detail template; Tasks 4-9 follow its exact structure and only restate what's unique to their class.

- [ ] **Step 1: Write the SKILL.md**

Frontmatter: `name: hunt-prototype-pollution`, `description:` one or two sentences stating what it covers (client-side and server-side JS prototype pollution via `__proto__`/`constructor.prototype` key injection through JSON merge, query-string parsing, or deep-clone utilities) and when to invoke (any endpoint that deep-merges attacker JSON into an object, any Node.js backend, any client-side framework reading query params into app state).

Body sections, matching `skills/hunt-ssrf/SKILL.md`'s shape:
- `## When to invoke` — concrete triggers: JSON body merge endpoints, query-string-to-object parsing (`qs`, `jQuery.extend`, lodash `_.merge`/`_.defaultsDeep`), any stack trace or error revealing a vulnerable merge library/version.
- Crown-jewel targets / impact ladder: client-side (DOM XSS via polluted `innerHTML` sink, CSP bypass), server-side (SSPP → RCE chains via `child_process`, or template-engine RCE via EJS/Pug/Handlebars reading a polluted option — research_notes/ and the repo's own `bb-local-toolkit`/`bug-bounty` skill descriptions already name this exact chain, consult them for the technique list).
- Payload/detection table: `__proto__[x]=y`, `constructor[prototype][x]=y`, and the JSON-body equivalents; a detection technique for confirming pollution landed (e.g. requesting a second endpoint and observing the injected property's effect, or a timing/behavior oracle when no direct reflection exists).
- `## Depth floor` — ≥25 distinct payload/sink combinations before declaring the surface exhausted.
- `## Evidence gate` — for client-side: the exact request + the polluted property's observable effect (console/DOM state) before `report_finding`; for server-side: the request plus a reproducible side effect (not just "the property was set" — show it changed real behavior). Tag `vulnclass: prototype-pollution`.
- `## Stop-authority` — same disclaimer pattern as `hunt-ssrf`'s.

- [ ] **Step 2: Lint it**

Run: `python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py skills/hunt-prototype-pollution/SKILL.md`
Expected: exit 0, no output (clean).

- [ ] **Step 3: Check the running description-byte total**

Add this skill's `description` length to Task 2's recorded total; confirm still under budget (flag loudly in the commit message if not, so Task 10 knows to revisit Task 2's trim list before binding).

- [ ] **Step 4: Commit**

```bash
git add skills/hunt-prototype-pollution/SKILL.md
git commit -m "feat(skill): hunt-prototype-pollution methodology"
```

---

### Task 4: Author `hunt-mass-assignment`

**Files:**
- Create: `skills/hunt-mass-assignment/SKILL.md`

**Interfaces:** Same pattern as Task 3; independent deliverable.

- [ ] **Step 1: Write the SKILL.md**

`name: hunt-mass-assignment`. Covers: over-posting/mass-assignment where a request body sets fields the client shouldn't control (`role`, `isAdmin`, `price`, `balance`, `userId` on someone else's resource) because the backend binds the whole JSON body to a model/struct without an allowlist. When to invoke: any create/update endpoint accepting a JSON object, especially ones backed by an ORM with implicit field binding (Rails strong-params gaps, Django ModelForm, ASP.NET model binding, Spring `@ModelAttribute`). Technique: diff the documented/expected fields against the full model schema (via API docs, OpenAPI spec, client JS, or GraphQL introspection) and send the extra fields; try nested-object and array-of-objects variants since some frameworks only allowlist the top level. Depth floor ≥25 (field × endpoint combinations). Evidence gate: the request showing the extra field, and the subsequent read/behavior proving it was actually applied (not just accepted silently). Stop-authority: standard disclaimer.

- [ ] **Step 2: Lint it** — same command as Task 3 Step 2, this file.

- [ ] **Step 3: Check running byte total** — same as Task 3 Step 3.

- [ ] **Step 4: Commit**

```bash
git add skills/hunt-mass-assignment/SKILL.md
git commit -m "feat(skill): hunt-mass-assignment methodology"
```

---

### Task 5: Author `hunt-deserialization`

**Files:**
- Create: `skills/hunt-deserialization/SKILL.md`

**Interfaces:** Same pattern as Task 3; independent deliverable.

- [ ] **Step 1: Write the SKILL.md**

`name: hunt-deserialization`. Covers insecure deserialization across ecosystems: Java (`ObjectInputStream`, common gadget chains — ysoserial families), PHP (`unserialize()` + magic methods `__wakeup`/`__destruct`), Python (`pickle.loads`, `yaml.load` without `SafeLoader`), .NET (`BinaryFormatter`), Ruby (`Marshal.load`, `YAML.load`). When to invoke: any endpoint accepting a base64/binary blob in a cookie, header, or hidden field; any stack trace naming a deserialization library; any "remember me" / session-token format that looks non-JSON. Technique: fingerprint the serialization format from a legitimate sample, identify the language/library and version, check for known gadget chains (ysoserial for Java, PHPGGC for PHP) appropriate to the libraries on the classpath/vendor dir if visible. Depth floor ≥25 (format/gadget-chain combinations attempted). Evidence gate: a non-destructive proof primitive (DNS/HTTP callback via a gadget chain, or a sleep/time-based primitive) — never a destructive gadget in production; mirror `fis-global-context`'s existing RCE non-intrusive-proof convention (DNS callback, whoami, hostname, echo) if operating under a similar engagement. Stop-authority: standard disclaimer.

- [ ] **Step 2: Lint it** — same command, this file.

- [ ] **Step 3: Check running byte total.**

- [ ] **Step 4: Commit**

```bash
git add skills/hunt-deserialization/SKILL.md
git commit -m "feat(skill): hunt-deserialization methodology"
```

---

### Task 6: Author `hunt-ssti`

**Files:**
- Create: `skills/hunt-ssti/SKILL.md`

**Interfaces:** Same pattern as Task 3; independent deliverable. Note: this repo's available-skills listing already shows a Claude-Code-side `ssti-hunter` agent and `/hunt-ssti` skill exist for the *operator's own* bug-bounty workflow — this new file is a **separate, ARTEX-internal** skill for ARTEX's own worker/pentest agents (per the W-Skill design doc's finding that these are two distinct loading mechanisms). Do not conflate the two or assume shared content; author this one fresh against the ARTEX skill-loader format.

- [ ] **Step 1: Write the SKILL.md**

`name: hunt-ssti`. Covers Server-Side Template Injection across engines: Jinja2/Twig (`{{7*7}}` polyglot detection → sandbox escape via `__class__`/`__mro__` chains), FreeMarker/Velocity (Java), ERB/Handlebars (Ruby/Node), Thymeleaf. When to invoke: any feature rendering user input through a template (comment formatting, email templates, PDF generation, admin-customizable templates, PR-description rendering). Technique: polyglot detection payloads first (`${7*7}`, `{{7*7}}`, `#{7*7}`, `<%= 7*7 %>`) to fingerprint the engine from which one evaluates, then escalate per-engine toward code execution; distinguish parse-time errors from runtime evaluation (a key diagnostic per the engine's own domain knowledge — template engines often differ in how they fail when a payload is malformed vs. when it's valid-but-sandboxed). Depth floor ≥25 (polyglot + per-engine payload attempts). Evidence gate: the exact payload plus the evaluated-arithmetic or command-output proof in the response; never run a destructive command in production — prefer `whoami`/`echo`/sleep-timing. Stop-authority: standard disclaimer.

- [ ] **Step 2: Lint it** — same command, this file.

- [ ] **Step 3: Check running byte total.**

- [ ] **Step 4: Commit**

```bash
git add skills/hunt-ssti/SKILL.md
git commit -m "feat(skill): hunt-ssti methodology"
```

---

### Task 7: Author `hunt-xxe`

**Files:**
- Create: `skills/hunt-xxe/SKILL.md`

**Interfaces:** Same pattern as Task 3; independent deliverable.

- [ ] **Step 1: Write the SKILL.md**

`name: hunt-xxe`. Covers XML External Entity injection: classic file-read via `<!ENTITY>`, blind/OOB XXE via parameter entities + external DTD, SOAP services, SVG/DOCX/XLSX (Office Open XML is a zip of XML — upload-based XXE), and error-based exfiltration when direct reflection is unavailable. When to invoke: any endpoint accepting raw XML (`Content-Type: application/xml`/`text/xml`), SOAP services, SVG/image upload that gets parsed, Office-document upload/export features, any JSON API with an "import from XML" legacy fallback. Technique ladder: classic in-band entity read first; if the parser rejects internal DTD subsets, escalate to OOB via parameter entities fetching an external DTD that exfiltrates via error messages or a callback (use `oob-probe` skill if available for the callback). Depth floor ≥25 (entity-technique × target-file or target-port combinations, including blind variants). Evidence gate: the file content or OOB interaction captured; mirror `hunt-ssrf`'s OOB confirmation guidance for the blind case since the callback mechanism is the same. Stop-authority: standard disclaimer.

- [ ] **Step 2: Lint it** — same command, this file.

- [ ] **Step 3: Check running byte total.**

- [ ] **Step 4: Commit**

```bash
git add skills/hunt-xxe/SKILL.md
git commit -m "feat(skill): hunt-xxe methodology"
```

---

### Task 8: Author `hunt-cors`

**Files:**
- Create: `skills/hunt-cors/SKILL.md`

**Interfaces:** Same pattern as Task 3; independent deliverable. Note: this repo's Claude-Code-side skill list already has a separate `cors-hunter` agent for the operator's own bug-bounty workflow — same caveat as Task 6: this is a fresh, ARTEX-internal file, not a port of that content.

- [ ] **Step 1: Write the SKILL.md**

`name: hunt-cors`. Covers CORS misconfiguration: origin reflection (`Access-Control-Allow-Origin` echoing the request's `Origin` header) combined with `Access-Control-Allow-Credentials: true`, null-origin bypass (sandboxed iframe / `file://` tricks), regex misconfigurations (`*.victim.com` matching `evilvictim.com` or subdomain takeover of an allowed origin), and pre-flight bypass via simple-request method/header tricks. When to invoke: any API serving sensitive data over `fetch`/XHR from a browser context, any endpoint whose response includes `Access-Control-Allow-Origin`. Technique: send a request with an attacker-controlled `Origin` header (a domain you control, a `null` origin, a crafted subdomain matching a loose regex) and check whether it's reflected AND whether credentials are allowed — the combination is what makes it exploitable, not either alone. Depth floor ≥25 (origin-variant × endpoint combinations). Evidence gate: the request/response pair showing the reflected origin + `Allow-Credentials: true`, plus a PoC HTML page (hosted, not executed against production data) demonstrating cross-origin credentialed read. Stop-authority: standard disclaimer.

- [ ] **Step 2: Lint it** — same command, this file.

- [ ] **Step 3: Check running byte total.**

- [ ] **Step 4: Commit**

```bash
git add skills/hunt-cors/SKILL.md
git commit -m "feat(skill): hunt-cors methodology"
```

---

### Task 9: Author `hunt-subdomain-takeover`

**Files:**
- Create: `skills/hunt-subdomain-takeover/SKILL.md`

**Interfaces:** Same pattern as Task 3; independent deliverable. Note: a Claude-Code-side `subdomain-takeover` agent already exists for the operator's own workflow (and the skill linter in `skills/chain-toolkit/scripts/tests/_lint_skill.py` explicitly allow-lists `subdomain-takeover` as "a real vuln class, not a penred agent" per an existing commit — confirm this new file's `name` doesn't collide/trip that linter rule; it shouldn't, since the linter only forbids `<word>-hunter` patterns and a short explicit list, not this name).

- [ ] **Step 1: Write the SKILL.md**

`name: hunt-subdomain-takeover`. Covers dangling DNS records pointing at unclaimed cloud resources: CNAME to an unclaimed S3 bucket / Azure blob / GitHub Pages / Heroku app / Fastly service / other CNAME-based SaaS, and dangling NS delegation. When to invoke: any subdomain enumeration result where a CNAME resolves to a known vulnerable-service fingerprint (consult a standard fingerprint list — e.g. `github.io`, `s3.amazonaws.com`, `herokuapp.com`, `azurewebsites.net`, `fastly.net`, `pages.github.com`, `shopify.com`, `wordpress.com` — each with its own "unclaimed" error-page signature). Technique: resolve CNAME chains for every discovered subdomain, match against known fingerprints, attempt (where policy allows) a non-destructive claim-proof (register the exact identifier on the free tier of the pointed-at service and show it resolves) — many bounty programs restrict actually claiming; default to **reporting the fingerprint match + CNAME + error-page evidence** without claiming unless the engagement's scope explicitly allows it (mirror `fis-global-context`'s posture on destructive/scope-sensitive actions: verify authorization before an active claim). Depth floor ≥25 (subdomain × service-fingerprint combinations, i.e. this is naturally volume-driven so the floor is almost always exceeded by normal enumeration — state that explicitly rather than padding attempts). Evidence gate: the CNAME record, the resolved error page/fingerprint, and (if claimed under explicit authorization) the proof-of-claim. Stop-authority: standard disclaimer.

- [ ] **Step 2: Lint it** — same command, this file.

- [ ] **Step 3: Check running byte total** — this is the 7th and last new skill; confirm the FULL running total (Task 2's baseline + all 7 new descriptions) is still under the ~8000-char budget for whichever agent(s) Task 10 binds them to. If over budget, trim descriptions across Tasks 3-9's files now, before Task 10 binds them (re-run each trimmed file's lint check).

- [ ] **Step 4: Commit**

```bash
git add skills/hunt-subdomain-takeover/SKILL.md
git commit -m "feat(skill): hunt-subdomain-takeover methodology"
```

---

### Task 10: Wire the phase-3 setup seed, tests, and docs

**Files:**
- Create: `scripts/setup-enhance-phase3.sh`
- Create: `scripts/tests/stub_enhance_phase3.py`
- Create: `scripts/tests/test_setup_enhance_phase3.sh`
- Create: `scripts/tests/test_phase3_skill_refs.sh`
- Create: `docs/enhance-phase3.md`
- Modify: `docs/enhance-phase2.md` (remove the 7 classes from its "Phase-2 scope note" deferred list, pointing at phase 3 instead)

**Interfaces:**
- Consumes: the 7 skill names from Tasks 3-9 (`hunt-prototype-pollution`, `hunt-mass-assignment`, `hunt-deserialization`, `hunt-ssti`, `hunt-xxe`, `hunt-cors`, `hunt-subdomain-takeover`); the exact patterns in `scripts/setup-enhance-phase2.sh`, `scripts/tests/stub_enhance_phase2.py`, `scripts/tests/test_setup_enhance_phase2.sh`, `scripts/tests/test_phase2_skill_refs.sh`, `docs/enhance-phase2.md` as direct clones (same structure, same login/merge logic, same assertion shapes) with phase-2's skill list replaced by phase-3's.
- Produces: nothing further consumed by other tasks — this is the workstream's final integration deliverable.

- [ ] **Step 1: Clone the setup script**

Copy `scripts/setup-enhance-phase2.sh` to `scripts/setup-enhance-phase3.sh`, replacing `WORKER_SKILLS`/`PENTEST_SKILLS`'s JSON array with the 7 new names (keep the exact same login/merge/PUT structure — this script has no class-specific logic to change). Update the header comment to say "Phase-3" and list the new skills. Make it executable (`chmod +x`).

- [ ] **Step 2: Clone the stub server**

Copy `scripts/tests/stub_enhance_phase2.py` to `scripts/tests/stub_enhance_phase3.py`. Update its `STATE` dict's prior-skill fixtures to a plausible pre-existing set (e.g. `["hunting-methodology", "hunt-ssrf"]`, mirroring phase 2's own stub) — the exact prior set doesn't matter functionally, only that Step 3's test can assert it's preserved after the merge.

- [ ] **Step 3: Write the failing seed test, then make it pass**

Copy `scripts/tests/test_setup_enhance_phase2.sh` to `scripts/tests/test_setup_enhance_phase3.sh`, updating: the `SCRIPT`/`STUB` paths to the phase-3 files, `$NEW` to the 7 phase-3 skill names, and `assert_agent`'s "prior skill" argument to match whatever Step 2's stub fixture uses.

Run: `bash scripts/tests/test_setup_enhance_phase3.sh`
Expected (before Step 1/2 are correct): FAIL — this proves the test actually exercises the new script/stub pair rather than accidentally passing against stale phase-2 files. If it fails only because of a path/name mismatch (not a real logic gap), fix the clone, not the test's assertions.

Once Steps 1-2 are correct: re-run.
Expected: `ALL TESTS PASSED`, exit 0.

- [ ] **Step 4: Write and run the reference-integrity test**

Copy `scripts/tests/test_phase2_skill_refs.sh` to `scripts/tests/test_phase3_skill_refs.sh`, updating the skill-name loop to the 7 phase-3 names.

Run: `bash scripts/tests/test_phase3_skill_refs.sh`
Expected: `ALL REFS PRESENT`, exit 0 (none of Tasks 3-9's skills reference a `references/<file>.md` that doesn't exist — if any task's prose mentioned a reference file informally without creating it, this step catches it; fix by either creating the referenced file or removing the dangling mention).

- [ ] **Step 5: Run the lint pass over all 7 files once more, together**

```bash
for s in hunt-prototype-pollution hunt-mass-assignment hunt-deserialization hunt-ssti hunt-xxe hunt-cors hunt-subdomain-takeover; do
  python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py "skills/$s/SKILL.md" || echo "LINT FAIL: $s"
done
```
Expected: no `LINT FAIL` lines.

- [ ] **Step 6: Write `docs/enhance-phase3.md`**

Mirror `docs/enhance-phase2.md`'s structure exactly: intro + design pointer (this plan's spec path), "What it adds" (the 7 skills with a one-line description each, the 3 required clauses restated), "One-command setup" (the phase-3 script invocation), "Manual UI equivalent", "Storage model: None added", "Validation" (the lint loop from Step 5, the seed test from Step 3, the reference-integrity test from Step 4), and an "Acceptance gate (operator-run, pending)" section matching phase 2's honesty about live-target validation being unconfirmed — do not claim real-world efficacy that hasn't been tested against a live vulnerable target.

- [ ] **Step 7: Update `docs/enhance-phase2.md`'s scope note**

Change its "Phase-2 scope note" paragraph (currently listing all 7 classes as deferred) to state they shipped in phase 3, with a pointer to `docs/enhance-phase3.md`.

- [ ] **Step 8: Commit**

```bash
git add scripts/setup-enhance-phase3.sh scripts/tests/stub_enhance_phase3.py scripts/tests/test_setup_enhance_phase3.sh scripts/tests/test_phase3_skill_refs.sh docs/enhance-phase3.md docs/enhance-phase2.md
git commit -m "feat(seed): phase-3 skill bindings (7 deferred exotic classes) + tests"
```
