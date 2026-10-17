# Skill discovery + binding — live-instance verification

Closes the open question behind Phase-2's "Acceptance gate (operator-run,
pending)" item 1 in `docs/enhance-phase2.md`, and the premise of the
`2026-10-18-w-skill-binding-and-expansion` plan: does ARTEX's own skill-loading
mechanism actually discover skill packs under `./skills/` (e.g. `hunt-ssrf`,
`hunt-idor`) when run the standard way, and does binding-by-name via
`PUT /api/agents/{agent}/visibility` actually take effect?

**Determination: confirmed working end-to-end. Zero code changes needed.**

One nuance surfaced during verification (documented below under "A wrinkle in
Step 1") — it is a correct, intentional property of `config.BaseDir()`, not a
bug, but it changes how you must run a *built* (non-`go run`) binary to get a
faithful test.

## Environment

- Worktree: `/Users/hiepck/Tools/ARTEX-EN/.worktrees/w-skill-binding` (branch
  `feat/w-skill-binding-and-expansion`).
- Go 1.27.1, darwin/arm64.
- Throwaway Postgres: `docker run -d --name artex-verify-pg -e POSTGRES_USER=artex_test
  -e POSTGRES_PASSWORD=artex_test -e POSTGRES_DB=artex_test -p 55433:5432 postgres:16`
  (port 55433 chosen to avoid the 55432 a sibling worktree may be using, and to
  avoid the unrelated `artex-en-postgres-1` / `decepticon-postgres` containers
  already running on this machine on other ports).
- `ARTEX_PG_DSN="postgres://artex_test:artex_test@localhost:55433/artex_test?sslmode=disable"`

## Step 1: stand up a live instance

First attempt, following the brief literally:

```bash
CGO_ENABLED=0 go build -o /tmp/artex-verify ./cmd/artex
ARTEX_PG_DSN=... /tmp/artex-verify -addr :18787
```

Startup log:

```
2026/10/18 01:15:27 [config] skill directory: /tmp/skills
```

**This is wrong for what we're trying to test** — but not a product bug. Per
`config/config.go`'s own doc comment, `BaseDir()` is the directory the
*running binary* lives in, not the caller's working directory (except for the
`go run` throwaway-binary special case, which falls back to CWD so dev
artifacts resolve against the project dir). A binary built straight into
`/tmp` is a real, non-`go-run` binary, so `BaseDir()` correctly anchors to
`/tmp`, and `SkillDir()` falls back to `/tmp/skills` — empty, since none of
this repo's skill packs live there. The brief's Step 1 assumed that running
`/tmp/artex-verify` from the repo root would make `BaseDir()` resolve to the
repo root; that assumption doesn't match the documented (and intentional)
behavior for a built binary.

This is also how production deployment actually works, by design:
`Dockerfile` places the binary at `/app/artex` and separately
`COPY skills/ /app/skills/` right next to it; `build.sh`'s `--release`
packaging step does `cp -R skills "$package_root/"` alongside the binary for
the same reason. `.gitignore` reserves `/artex` (and `/artex.exe`) at the repo
root — the standard dev/build output path — confirming the intended pattern is
"binary and `skills/` live in the same directory," not "binary resolves paths
relative to whatever directory it's invoked from."

**Corrected Step 1** — build the binary directly into the repo root (matching
the `.gitignore`-reserved `/artex` path and the Docker/release layout), so
`BaseDir()` genuinely anchors there:

```bash
cd /Users/hiepck/Tools/ARTEX-EN/.worktrees/w-skill-binding
CGO_ENABLED=0 go build -o ./artex-verify-bin ./cmd/artex
ARTEX_PG_DSN="postgres://artex_test:artex_test@localhost:55433/artex_test?sslmode=disable" \
  ./artex-verify-bin -addr :18787 -proxy 127.0.0.1:18788
```

Startup log (relevant lines):

```
2026/10/18 01:16:56 [config] skill directory: /Users/hiepck/Tools/ARTEX-EN/.worktrees/w-skill-binding/skills
2026/10/18 01:16:56 [auth] a new JWT key was written to /Users/hiepck/Tools/ARTEX-EN/.worktrees/w-skill-binding/jwt.key
2026/10/18 01:16:56 [engine] no LLM provider configured — engine idle until set via /api/llm or env
2026/10/18 01:16:56 Penred dev backend listening on :18787 (data=.../data, workers=3)
```

`skillDir` now correctly resolves to this repo's `./skills/`.

## Step 2: first-run bootstrap

`CLAUDE.md` says "first run serves `/setup`" — that's the frontend route; the
no-tag stub build has no UI, so the API sequence underneath it is:

```bash
$ curl -s http://127.0.0.1:18787/api/auth/status
{"initialized":false}

$ curl -s -X POST http://127.0.0.1:18787/api/auth/init \
    -H 'Content-Type: application/json' -d '{"password":"VerifyPass123!"}'
{"token":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...."}
```

`GET /api/auth/status` reports whether the admin password is set;
`POST /api/auth/init` sets it the first time (403 if already set) and returns
a usable JWT directly — no separate login call needed on first run. (The setup
scripts' `POST /api/auth/login` with `username:"Penred"` is the path for an
*already-initialized* instance, confirmed working in Step 4 below via
`setup-enhance-phase2.sh`, which does exactly that login.)

## Step 3: confirm skill discovery

```bash
$ TOKEN=<token from Step 2>
$ curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18787/api/skills \
    | jq -r '.skills[].name' | sort
api-recon
chain-hunting
chain-toolkit
client-verify
hunt-business-logic
hunt-cache-poison
hunt-graphql-authz
hunt-idor
hunt-info-disclosure
hunt-oauth
hunt-race-conditions
hunt-rce
hunt-smuggling
hunt-ssrf
hunt-xss
hunting-methodology
intel-dupcheck
oob-probe
playwright-cli
scopesentry
validation-gate
```

Note the real response shape is `{"skills":[...]}`, not a bare array — the
brief's `jq -r '.[].name'` needed adjusting to `.skills[].name`, which the
brief explicitly anticipated ("or whatever shape the endpoint actually
returns").

Diffed directly against the filesystem:

```bash
$ diff <(ls skills/ | sort) \
       <(curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18787/api/skills \
           | jq -r '.skills[].name' | sort)
$ echo $?
0   # identical — all 21 directories under ./skills/ are discovered, 1:1, nothing missing/extra
```

## Step 4: confirm binding end to end

```bash
$ ARTEX_URL=http://127.0.0.1:18787 ARTEX_ADMIN_PASSWORD='VerifyPass123!' \
    bash scripts/setup-enhance-phase2.sh
bound Phase-2 skills to worker (merged)
bound Phase-2 skills to pentest (merged)
Phase-2 skill binding seed complete.

$ curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18787/api/agents/worker/visibility | jq .
{
  "mcp": null,
  "skill": [
    "api-recon",
    "hunt-cache-poison",
    "hunt-graphql-authz",
    "hunt-race-conditions",
    "hunt-smuggling",
    "hunt-ssrf"
  ]
}

$ curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18787/api/agents/pentest/visibility | jq .
{
  "mcp": null,
  "skill": [
    "api-recon",
    "hunt-cache-poison",
    "hunt-graphql-authz",
    "hunt-race-conditions",
    "hunt-smuggling",
    "hunt-ssrf"
  ]
}
```

All 5 Phase-2 skill names (`hunt-ssrf`, `hunt-cache-poison`,
`hunt-race-conditions`, `hunt-graphql-authz`, `hunt-smuggling`) are present on
both `worker` and `pentest`, alongside a pre-existing `api-recon` binding that
was already there (merge preserved it, as designed — no destructive overwrite).

### Confirmed this is the same path the engine actually uses

`server/assembly.go`'s `wireAgentAugment` — the function that wires an agent's
runtime tool set on each task — reads `pg.AgentSkillNames(a.ID)` (the exact
column `pgSetAgentVisibility`/`SetAgentSkillVisibility` writes), loads
`skill.LoadDir(skillDir)` (the exact `skillDir` the server was started with,
from `config.SkillDir()`), and matches by `filepath.Base(s.Dir)` against the
bound names:

```go
if names, _ := pg.AgentSkillNames(a.ID); len(names) > 0 {
    ...
    if allReg, err := skill.LoadDir(skillDir); err == nil && allReg != nil {
        reg = skill.NewRegistry()
        for _, s := range allReg.List() {
            // match by directory name (Base of Dir), not by skill display Name
            if s.Dir != "" && nameSet[filepath.Base(s.Dir)] {
                reg.Add(s)
            }
        }
    }
}
```

So the `GET .../visibility` response verified above isn't just a config-table
echo — it's reading the identical data the engine consumes to build an agent's
Skill meta-tool on the next task. There is no separate "what the UI shows" vs.
"what the engine loads" path to drift apart.

## Step 5: fix needed?

No. Both steps succeeded once Step 1 placed the binary where `config.go`
documents it will look (next to itself, matching the Docker/release deploy
layout) — which is also what the plan's stated "standard build"
(`go build -tags embedui -o artex ./cmd/artex` from the repo root, landing at
the `.gitignore`-reserved `/artex`) naturally does. No change to
`config/config.go`, the setup scripts, or any doc was required.

## Step 6: teardown

```bash
pkill -f "./artex-verify-bin"
docker rm -f artex-verify-pg
rm -f artex-verify-bin jwt.key
rm -rf data
```

`git status` confirmed a clean working tree afterward (only this findings doc
is new).

## Takeaway for later tasks in this plan

- New skill packs authored under `./skills/<name>/SKILL.md` will be discovered
  automatically by `GET /api/skills` and bindable via
  `PUT /api/agents/{agent}/visibility` with no extra wiring, **provided the
  running binary and the `skills/` directory live side by side** — true for
  `go run` (dev), the repo-root `go build -o artex` build, and the
  Docker/release packaging, but not for an ad hoc build dropped in an
  unrelated directory (e.g. `/tmp`) without also setting `ARTEX_SKILL_DIR` or
  `skill_dir` in `config.json`.
- If anyone scripts a similar live-instance check later: build the verify
  binary into the repo root (or set `ARTEX_SKILL_DIR` explicitly), not into
  `/tmp`, or the `skillDir` log line will quietly point at an empty directory
  and look like a false "skills missing" failure that isn't one.

## Task 2: skill-description byte-budget audit

Closes Task 2 of the `2026-10-18-w-skill-binding-and-expansion` plan: of the
skills actually bound to `worker`/`pentest` right now, how much of the
`Skill` meta-tool's render budget do their `description:` frontmatter fields
use, and how much headroom is left before Tasks 3-9 add 7 more?

### Environment (fresh instance, independent of Task 1's teardown)

Same recipe as Task 1, with a different container/binary/port so nothing
could collide with a lingering Task-1 resource:

```bash
docker run -d --name artex-verify-pg2 -e POSTGRES_USER=artex_test2 \
  -e POSTGRES_PASSWORD=artex_test2 -e POSTGRES_DB=artex_test2 \
  -p 55434:5432 postgres:16

cd /Users/hiepck/Tools/ARTEX-EN/.worktrees/w-skill-binding
CGO_ENABLED=0 go build -o ./artex-verify-bin2 ./cmd/artex
ARTEX_PG_DSN="postgres://artex_test2:artex_test2@localhost:55434/artex_test2?sslmode=disable" \
  ./artex-verify-bin2 -addr :18797 -proxy 127.0.0.1:18798
```

Startup log confirmed `skillDir` resolved to this repo's `./skills/` (same
fix from Task 1 — binary built into the repo root, not `/tmp`).

### Step 1: bootstrap + bind, read the live visibility lists

```bash
curl -s http://127.0.0.1:18797/api/auth/status
# {"initialized":false}
curl -s -X POST http://127.0.0.1:18797/api/auth/init -H 'Content-Type: application/json' \
  -d '{"password":"VerifyPass123!"}'
# {"token":"...<JWT>..."}

ARTEX_URL=http://127.0.0.1:18797 ARTEX_ADMIN_PASSWORD='VerifyPass123!' \
  bash scripts/setup-enhance-phase2.sh
# bound Phase-2 skills to worker (merged)
# bound Phase-2 skills to pentest (merged)
# Phase-2 skill binding seed complete.

curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18797/api/agents/worker/visibility | jq .
curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:18797/api/agents/pentest/visibility | jq .
```

Both returned the identical list (matching Task 1's finding — the
`builtinSkillVisibility` seed in `db/db.go` binds `api-recon` to
`auto`/`pentest`/`worker` by default on a fresh DB, on top of which
`setup-enhance-phase2.sh` merges in 5 more):

```json
{
  "mcp": null,
  "skill": [
    "api-recon",
    "hunt-cache-poison",
    "hunt-graphql-authz",
    "hunt-race-conditions",
    "hunt-smuggling",
    "hunt-ssrf"
  ]
}
```

**6 skills are currently bound, identically, to both `worker` and `pentest`.**
No other skill in the repo (including `hunt-idor`, `chain-hunting`,
`validation-gate`, etc.) is bound to either agent today — they exist under
`./skills/` and are discoverable via `GET /api/skills`, but are invisible to
the `Skill` meta-tool until an operator opts them in via
`PUT /api/agents/{agent}/visibility`.

### Step 2: measure each bound skill's `description:` length

Per `norma@v0.4.3`'s `skill/skill.go` (the vendored module backing ARTEX's
`Skill` meta-tool — read directly from the Go module cache to confirm the
exact mechanism, not re-derived from memory):

```go
const (
	maxSkillListingChars = 8000
	maxSkillDescChars    = 500
)
```

`listing()` renders, for each skill the registry was filtered down to (per
agent, via the `AgentSkillNames` → `LoadDir` → filter-by-name path confirmed
in Task 1's Step 4), a line `"\n- " + name + ": " + desc`; any single
`desc` over 500 is clipped to `desc[:500] + "…"`; the running total `used`
(byte length of every rendered line so far) is capped at 8000 — past that,
remaining skills in the list degrade to name-only but are never dropped.
Both constants are **byte** lengths (Go's `len(string)` counts UTF-8 bytes,
not Unicode codepoints) — this matters here because two descriptions below
contain an em dash (`—`, 3 bytes, 1 character), so byte-length and
character-length differ by a small amount per occurrence.

Measured directly against the live-bound 6 (`skills/<name>/SKILL.md`
frontmatter, one command per file, cross-checked with both a `) chars"`
awk one-liner and a Python UTF-8-byte count):

| skill | desc chars | desc bytes | rendered line bytes (`"\n- name: desc"`) |
| --- | ---: | ---: | ---: |
| `api-recon` | 60 | 60 | 74 |
| `hunt-smuggling` | 257 | 259 | 278 |
| `hunt-race-conditions` | 262 | 264 | 289 |
| `hunt-cache-poison` | 280 | 282 | 304 |
| `hunt-graphql-authz` | 291 | 293 | 316 |
| `hunt-ssrf` | 302 | 304 | 318 |
| **Total (worker)** | **1452** | **1462** | **1579** |
| **Total (pentest)** | **1452** | **1462** | **1579** |

(Worker and pentest totals are identical and computed separately per the
brief's instruction — they happen to be the same number here only because
both agents currently have the exact same bound-skill set, not because the
`Skill` tool pools them.)

### Step 3: compare against budget, flag outliers

- **No currently-bound skill individually exceeds the ~500-char/byte
  per-skill cap.** The largest is `hunt-ssrf` at 304 bytes — well inside
  the 500-byte `maxSkillDescChars` clip threshold, so no description is
  truncated with `…` in the rendered listing today.
- **Per-agent total is 1,579 rendered bytes against the 8,000-byte
  `maxSkillListingChars` budget** — **≈6,421 bytes of headroom** before the
  listing would start degrading remaining skills to name-only, for *each*
  of `worker` and `pentest` independently.
- **Headroom for Tasks 3-9:** 7 new skills are coming
  (`hunt-prototype-pollution`, `hunt-mass-assignment`,
  `hunt-deserialization`, `hunt-ssti`, `hunt-xxe`, `hunt-cors`,
  `hunt-subdomain-takeover`, per the plan's own Task 3-9 headers — none of
  these is `hunt-idor`). If each stays under the ~500-char/byte
  per-skill guidance like the 6 above (max observed ~320 rendered bytes
  each), 7 more skills add at most **≈2,240 rendered bytes**, landing the
  per-agent total at **≈3,820 of 8,000 bytes — still well under budget**,
  *provided* each new skill is bound to the same agent(s) as the existing
  six and each individual task re-checks its own addition against this
  table before moving to the next, per the plan's Task 3 Step 3 interface.

### Outlier found and trimmed: `hunt-idor` (not currently bound, but flagged by the plan's own research)

`hunt-idor` is **not** in either agent's current visibility list (confirmed
above), so it does not count toward the totals/headroom figures. It was
still checked because the plan text explicitly flags it ("the earlier
research flagged `hunt-idor`'s as the known outlier, ~1400 chars") as a
latent risk for whenever it (or a skill like it) gets bound later — trimming
it now is cheap and removes a known landmine before Tasks 3-9 touch
anything.

Measured `hunt-idor`'s original `description:` field directly (not via the
500-char-cap-aware `listing()` renderer, since it isn't registry-filtered in
today — just the raw frontmatter):

- **Before: 904 chars / 908 bytes** — about 1.8x the ~500 budget (the plan's
  "~1400 chars" estimate was evidently rough; the actual figure measured
  here is 904/908). The field stuffed in a 1,117-report corpus citation and
  a run of ten-plus CVE/GHSA identifiers (OneUptime, Zitadel ×2, Inforcer,
  Apache Answer, Indico, FastGPT/WeKnora/Paperclip) that already live, in
  full narrative form, in the skill's own body (`Crown Jewel Targets`,
  `Common Root Causes`, `Bypass Techniques` sections) — so trimming the
  frontmatter line loses no unique information, only the duplicate summary.

Trimmed to the essential trigger criteria (what it covers, when to invoke),
dropping the report-count/CVE-list narrative since it's already fully
preserved in the body:

```
Hunting skill for Insecure Direct Object Reference / Broken Object Level
Authorization (BOLA — OWASP API1:2023). Covers multi-tenant header bypass,
GraphQL field-level/nested-object pivots, path-vs-body ID mismatch, UUIDv1
prediction, mass assignment, and cross-tenant AI/agent access. Use when
hunting IDOR/BOLA on web apps, APIs, GraphQL, multi-tenant SaaS, mobile,
automotive/IoT, or AI/ML inference servers.
```

- **After: 411 chars / 413 bytes** — under the 500-byte budget with room to
  spare.

Verified against the live instance (still running at that point) that the
trimmed description is exactly what `GET /api/skills` serves — skills are
read fresh off disk per request, no restart needed — and against the repo's
own skill linter:

```bash
$ python3 -I skills/chain-toolkit/scripts/tests/_lint_skill.py skills/hunt-idor/SKILL.md
$ echo $?
0
```

No other file was modified; the six currently-bound skills' descriptions
were left as-is (none needed trimming).

### Step 4: teardown

```bash
pkill -f "./artex-verify-bin2"
docker rm -f artex-verify-pg2
rm -f artex-verify-bin2 jwt.key
rm -rf data
```

`git status` afterward showed exactly one modified file
(`skills/hunt-idor/SKILL.md`) and this doc's edit — no stray build
artifacts, containers, or data directories left behind.

### Takeaway for Tasks 3-9

- Use the per-skill byte counts above as a sanity ceiling: stay well under
  500 bytes per `description:` field (the six currently-bound skills range
  60-304 bytes; aim for that range, not the cap).
- Current per-agent rendered total is 1,579 of 8,000 bytes. Budget roughly
  300-450 rendered bytes per new skill description and re-sum before moving
  to the next task, as each task's own Step 3 already requires.
- `hunt-idor` is fixed now (413 bytes) — if a future task considers binding
  it to `worker`/`pentest`, it no longer needs separate trimming.
