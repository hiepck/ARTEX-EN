#!/usr/bin/env bash
# Sync ARTEX-EN with the upstream Chinese repo (Autumn-27/ARTEX).
#
# Usage:
#   scripts/sync-upstream.sh            fetch + merge upstream/main, then scan for untranslated Chinese
#   scripts/sync-upstream.sh --build    same, and if everything is clean, run scripts/build-en.sh
#
# Exit codes: 0 = up to date & fully translated; 2 = merge conflicts to resolve;
#             3 = merged OK but Chinese strings still need translating.
set -euo pipefail
cd "$(dirname "$0")/.."

BUILD=0
[ "${1:-}" = "--build" ] && BUILD=1

git remote get-url upstream >/dev/null 2>&1 || {
  echo "ERROR: no 'upstream' remote. Run:  git remote add upstream https://github.com/Autumn-27/ARTEX.git" >&2
  exit 1
}

# 1. Refuse to run on a dirty tree — a merge would mix changes together.
if [ -n "$(git status --porcelain)" ]; then
  echo "ERROR: working tree is not clean — commit or stash your changes first." >&2
  git status --short >&2
  exit 1
fi

# 2. Fetch the latest Chinese code.
echo "==> Fetching upstream..."
git fetch upstream --tags --prune

# 3. Merge (skip if already contained).
if git merge-base --is-ancestor upstream/main HEAD; then
  echo "==> Already up to date with upstream/main."
else
  LABEL="$(git describe --tags upstream/main 2>/dev/null || git rev-parse --short upstream/main)"
  echo "==> Merging upstream/main (${LABEL})..."
  if ! git merge upstream/main -m "Merge upstream ${LABEL}"; then
    echo
    echo "!! MERGE CONFLICTS. Resolution policy for this repo:"
    echo "   - take upstream's NEW logic and structure;"
    echo "   - keep / re-apply the English translation of user-facing strings and comments."
    echo "   Conflicted files:"
    git diff --name-only --diff-filter=U | sed 's/^/     /'
    echo "   When done:  git add <files> && git commit      (to abandon: git merge --abort)"
    echo "   Tip: open Claude Code in this repo and ask it to resolve the conflicts with the policy above."
    exit 2
  fi
fi

# 4. Scan for Chinese characters outside the intentional-data allowlist.
echo "==> Scanning for untranslated Chinese strings..."
CJK='[\x{4e00}-\x{9fff}]'
HITS="$(grep -rnP "$CJK" . \
  --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=.next \
  --exclude-dir=out --exclude-dir=dist --exclude-dir=data \
  --exclude='*.sum' --exclude='*.png' --exclude='*.jpg' --exclude='*.webp' \
  --exclude=.translation-ignore \
  2>/dev/null | sed 's|^\./||' || true)"

IGNORE_LIST="$(grep -vE '^\s*(#|$)' .translation-ignore 2>/dev/null || true)"
NEEDED=""; NEEDED_N=0; IGNORED_N=0
while IFS= read -r line; do
  [ -z "$line" ] && continue
  path="${line%%:*}"
  if printf '%s\n' "$IGNORE_LIST" | grep -qxF "$path"; then
    IGNORED_N=$((IGNORED_N + 1))
  else
    NEEDED="${NEEDED}${line}"$'\n'
    NEEDED_N=$((NEEDED_N + 1))
  fi
done <<< "$HITS"

if [ "$IGNORED_N" -gt 0 ]; then
  echo "    (${IGNORED_N} Chinese hits ignored as intentional data — see .translation-ignore)"
fi

if [ "$NEEDED_N" -gt 0 ]; then
  echo
  echo "!! ${NEEDED_N} line(s) still in Chinese — translation needed:"
  printf '%s' "$NEEDED" | sed 's/^/   /'
  echo
  echo "   Open Claude Code in this repo and say:"
  echo "     \"Translate the remaining Chinese strings listed by scripts/sync-upstream.sh to English,"
  echo "      then commit.\"  Afterwards re-run:  scripts/sync-upstream.sh --build"
  exit 3
fi

echo "==> Clean: no untranslated Chinese. Ready to build."
echo "    Push when satisfied:  git push origin main"

if [ "$BUILD" = 1 ]; then
  echo
  exec scripts/build-en.sh
fi
