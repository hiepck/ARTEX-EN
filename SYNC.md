# Keeping this English fork in sync with the upstream Chinese repo

This repo (`ARTEX-EN`) is a translation fork of [`Autumn-27/ARTEX`](https://github.com/Autumn-27/ARTEX)
(the Chinese original, which is under active development). This document describes how updates
flow from upstream into this fork and how the English build is produced.

## Git layout

| Remote     | URL                                          | Role                                  |
| ---------- | -------------------------------------------- | ------------------------------------- |
| `origin`   | `git@github.com:hiepck/ARTEX-EN.git`         | this fork (private), push here        |
| `upstream` | `https://github.com/Autumn-27/ARTEX.git`     | Chinese original, fetch-only          |

`main` contains upstream's history plus translation commits on top. Syncing is a plain
`git merge upstream/main` — no force-push, no rebase, history stays linear-ish and auditable.

## Update workflow (one command)

```bash
scripts/sync-upstream.sh            # fetch + merge + scan for untranslated Chinese
scripts/sync-upstream.sh --build    # ...and build the Docker image when everything is clean
```

The script will:

1. **Refuse to run on a dirty tree** (commit or stash first).
2. **Fetch and merge `upstream/main`.**
   - On conflicts it stops (exit 2) and prints the resolution policy below.
3. **Scan the repo for Chinese characters** (`\p{Han}` in source files), skipping the
   intentional-data paths listed in `.translation-ignore`.
   - Hits found → exit 3 with a `file:line:text` list to translate.
   - Clean → exit 0, ready to build.

After a successful sync: `git push origin main`.

### Conflict-resolution policy

When upstream touches files that were also translated, resolve each conflict as:

- **Take upstream's new logic and structure** (the Chinese repo is the source of truth for behavior).
- **Keep / re-apply the English translation** of user-facing strings and comments in the new code.
- Never mix: a resolved hunk must be *new code, fully in English*.

Then commit the merge and re-run `scripts/sync-upstream.sh` — the Chinese-string scan is the
safety net that catches strings which merged cleanly but are still untranslated.

### Translation policy

- New Chinese strings introduced by upstream are translated by hand or with Claude Code
  (open the repo and ask it to translate the scan output), then committed.
- `.translation-ignore` lists files where Chinese is **data, not text**: ICP filing numbers
  (`京ICP备…`), the legacy `[模型]` reason prefix that must keep matching pre-translation history,
  detection regexes for Chinese LLM-provider errors and CN login pages, and CJK test fixtures.
  Add new entries only for that kind of content.
- `CHANGELOG.md` stays in Chinese by convention of this fork.

## Building the English version

```bash
scripts/build-en.sh        # host needs only Docker; Go/Node run inside containers
```

Pipeline (same as `DOCKER.md`, but containerized so no host toolchain is required):

```
web/  --(node:22, npm ci && NEXT_EXPORT=1 next build)-->  web/out
web/out  --(copy)-->  server/webui/dist
server/webui/dist  --(golang:1.26, go build -tags embedui)-->  dist/amd64/artex
dist/amd64/artex  --(docker build)-->  image artex-en:latest
```

## Running

```bash
cp docker-compose.override.yml.example docker-compose.override.yml   # once: use artex-en:latest, never pull
cp .env.example .env                                                 # once: set POSTGRES_PASSWORD
docker compose up -d
```

Open http://localhost:8787 — first visit goes to `/setup`. Verify the build:

```bash
docker compose exec artex /app/artex -h     # help text must be in English
```

## Verification checklist after each sync

- [ ] `go build ./...` clean (the Go step of `build-en.sh` covers this)
- [ ] `scripts/sync-upstream.sh` exits 0 (no untranslated Chinese outside `.translation-ignore`)
- [ ] `docker compose up -d` then UI at :8787 renders in English
- [ ] `git push origin main`
