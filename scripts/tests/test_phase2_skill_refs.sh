#!/usr/bin/env bash
# Guards against dangling lazy-load pointers: every `references/<file>.md`
# mentioned by a Phase-2 skill must exist on disk.
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
fails=0
for s in hunt-ssrf hunt-cache-poison hunt-race-conditions hunt-graphql-authz hunt-smuggling; do
  skill="$ROOT/skills/$s/SKILL.md"
  [ -f "$skill" ] || { echo "FAIL: missing $skill"; fails=1; continue; }
  while IFS= read -r ref; do
    target="$ROOT/skills/$s/$ref"
    [ -f "$target" ] || { echo "FAIL: $s points at missing $ref"; fails=1; }
  done < <(grep -oE 'references/[A-Za-z0-9._-]+\.md' "$skill" | sort -u)
done
[ "$fails" -eq 0 ] && echo "ALL REFS PRESENT"
exit "$fails"
