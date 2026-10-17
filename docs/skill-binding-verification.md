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
