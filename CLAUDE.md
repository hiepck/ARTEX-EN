# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

ARTEX-EN is the **English translation fork** of [`Autumn-27/ARTEX`](https://github.com/Autumn-27/ARTEX),
an AI-driven autonomous penetration-testing system: a Go backend (event-driven LLM exploration engine
+ JSON HTTP API) with a Next.js / shadcn-ui frontend. Upstream is the Chinese original and the source
of truth for **behavior**; this fork only translates user-facing strings, comments, prompts, scripts,
and docs. The Go module path stays `github.com/Autumn-27/artex` — do **not** rename it.

### Translation-fork rules (read before editing)

- `main` is upstream history + translation commits on top. Syncing is `git merge upstream/main`
  (no rebase, no force-push). The full workflow is in `SYNC.md`.
- `scripts/sync-upstream.sh` fetches, merges, and scans for untranslated Chinese (`\p{Han}` in
  source). Exit 3 = untranslated strings remain (prints `file:line:text`); exit 0 = clean.
- `.translation-ignore` lists paths where Chinese is **data, not UI text** and must be left alone:
  ICP filing numbers, the legacy `[模型]` reason prefix (kept so pre-translation DB rows still match
  alongside the new `[model]`), Chinese LLM-gateway error regexes, CN login-page detection, and CJK
  test fixtures. Only add entries of that kind.
- `CHANGELOG.md` stays in Chinese by convention of this fork — do not translate it.
- When resolving a merge conflict: take upstream's new logic, re-apply the English translation of any
  strings/comments in it. A resolved hunk must be new code, fully in English.

## Build / run / test

The backend and the statically-exported frontend are **separate** unless you build with `-tags embedui`.

| Task | Command |
| --- | --- |
| Dev (split: backend :8787, proxy :8788, Next dev :5173 with `/api` proxied) | `./dev.sh` |
| Backend only | `go run ./cmd/artex -addr :8787 -proxy 127.0.0.1:8788` |
| Go tests (153 test files) | `go test ./...` |
| Single package / test | `go test ./agent/ -run TestName -v` |
| Single-binary build (current platform, embeds frontend) | `./build.sh` |
| All release targets + zips | `./build.sh --release` |
| Frontend dev / lint / format | `cd web && npm run dev` / `npm run check` / `npm run format` (Biome) |
| Frontend static export (feeds the Go embed) | `cd web && npm run build:static` → `web/out` |
| Containerized English build (no host Go/Node needed) | `scripts/build-en.sh` (produces `artex-en:latest`) |

`build.sh` does the full pipeline: `npm ci` + `build:static` → rsync `web/out` into
`server/webui/dist` → `CGO_ENABLED=0 go build -tags embedui ./cmd/artex`. The default (no-tag) Go
build ships a **stub** frontend handler (`server/webui_stub.go`); only `-tags embedui`
(`server/webui_embed.go`) embeds `server/webui/dist`. Set `ARTEX_SKIP_FRONTEND=1` to reuse an
existing `server/webui/dist`.

Go 1.26+ required. `Dockerfile` compiles **nothing** — it drops in a prebuilt `dist/<arch>/artex`.

## Runtime configuration

- **Database: PostgreSQL** (required). Resolution: `ARTEX_PG_DSN` env > `config.json` `database.dsn`
  > assembled `host/port/user/...` fields. See `config.example.json`. (Note: some older doc comments,
  e.g. in `cmd/artex/main.go`, still say "SQLite" — the `db` package is Postgres now; trust the `db`
  package and README.)
- **Config file path:** `ARTEX_CONFIG` env > `config.json` next to the executable.
- **Skills dir:** `ARTEX_SKILL_DIR` env > config `skill_dir` > `skills/` next to the binary. Each
  skill is a subdir with a `SKILL.md`.
- **LLM:** `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` (+ `ARTEX_LLM_*`), or set in the UI. Not in `config.json`.
- First run serves `/setup` to set the admin password.

## Architecture

`cmd/artex/main.go` is thin: self-update bootstrap → open stores → `server.New(...)` → serve. The
process exit code is a protocol: **exit 75** (`selfupdate.ExitRestart`) tells the supervising
`start.sh` to relaunch (how the UI one-click update swaps the binary); exit 0 stops the loop.
Running the binary directly (not via `start.sh`) breaks self-update restarts.

Go packages (each has a package doc comment — read it first):

- `agent` — the LLM exploration core: planner / worker / main-agent, tool catalog + execution,
  blackboard, compaction, finding workflow, side-questions. The behavioral heart of the system.
- `server` — HTTP API + orchestration: `Manager`/`Server`, task scheduler, engine, assets, findings,
  intercept/approvals, notifications, ScopeSentry sync, update, WebUI embed.
- `db` — PostgreSQL data source (asset graph, tasks, findings, activity).
- `enrich` — non-AI async asset auto-completion (dnsx resolve + HTTP probe via the recording proxy).
- `traffic` — request-recording proxy subsystem (the `-proxy` listener).
- `intercept` / `guard` — tool-call interception rules (PreToolUse, priority-ordered) and the safety
  boundary / audit layer.
- `llmpool` / `llmrec` — LLM failover/round-robin provider decorator, and a recording decorator.
- `mcphttp` — remote Streamable-HTTP / legacy-SSE MCP clients.
- `report` — renders deliverable Markdown reports from an exploration.
- `notify` — IM / email delivery of findings.
- `selfupdate` — one-click update: fetch, stage, boot-count, roll back a build that keeps dying.
- `config` — JSON config + env loading. `sidequestion` — immutable main-agent checkpoints.

`web/` is a Next.js app (App Router, static export via `NEXT_EXPORT=1`), styled with shadcn/ui + Tailwind,
graphs via AntV G6 / xyflow. Lint/format is **Biome** (`npm run check`), not ESLint/Prettier.

Design docs referenced by package comments (e.g. "docs §5/§10/§11", `asset-model-and-auto-linking-design.md`)
live upstream and are **not** vendored here; local `docs/` holds only `vulnerability-traffic-evidence.md`.
