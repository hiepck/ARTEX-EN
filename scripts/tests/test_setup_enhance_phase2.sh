#!/usr/bin/env bash
# Tests for setup-enhance-phase2.sh against a stateful recording stub.
#  1. No admin password -> nonzero exit, zero PUT.
#  2. worker merged     -> all 5 new skills, prior skill+mcp preserved, no dupes.
#  3. pentest merged    -> all 5 new skills, prior skill+mcp preserved, no dupes.
#  4. Idempotent        -> a second run produces an identical PUT body.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/../setup-enhance-phase2.sh"
STUB="$HERE/stub_enhance_phase2.py"
PORT=$(( (RANDOM % 20000) + 20000 ))
URL="http://127.0.0.1:$PORT"
LOG="$(mktemp)"; BODYLOG="$(mktemp)"; fails=0
export STUB_LOG="$LOG" STUB_BODYLOG="$BODYLOG"
python3 -I "$STUB" "$PORT" & STUB_PID=$!
disown "$STUB_PID" 2>/dev/null || true
cleanup() { kill "$STUB_PID" 2>/dev/null; rm -f "$LOG" "$BODYLOG"; }
trap cleanup EXIT
for _ in $(seq 1 50); do curl -fsS "$URL/api/agents/worker/visibility" >/dev/null 2>&1 && break; sleep 0.1; done
: > "$LOG"
NEW='hunt-ssrf hunt-cache-poison hunt-race-conditions hunt-graphql-authz hunt-smuggling'

# Last PUT body (JSON) for an agent, from the body log.
put_body() { grep -E "^PUT /api/agents/$1/visibility " "$BODYLOG" | tail -1 | sed -E "s#^PUT /api/agents/$1/visibility ##"; }
assert_agent() { # agent, prior-skill
  local agent="$1" prior="$2" body
  body="$(put_body "$agent")"
  [ -z "$body" ] && { echo "FAIL: $agent visibility never set"; fails=1; return; }
  for s in $NEW; do
    echo "$body" | jq -e --arg s "$s" '.skill | index($s) != null' >/dev/null 2>&1 \
      || { echo "FAIL: $s not bound to $agent"; fails=1; }
  done
  echo "$body" | jq -e --arg p "$prior" '.skill | index($p) != null' >/dev/null 2>&1 \
    || { echo "FAIL: prior skill $prior not preserved on $agent"; fails=1; }
  # each new skill appears exactly once (dedup)
  for s in $NEW; do
    n=$(echo "$body" | jq --arg s "$s" '[.skill[] | select(. == $s)] | length')
    [ "$n" = "1" ] || { echo "FAIL: $s appears $n times on $agent (want 1)"; fails=1; }
  done
  echo "$body" | jq -e '.mcp == [7]' >/dev/null 2>&1 \
    || { echo "FAIL: prior mcp not preserved on $agent"; fails=1; }
}

# Test 1: missing password -> fail, no PUT
( unset ARTEX_ADMIN_PASSWORD; ARTEX_URL="$URL" bash "$SCRIPT" ) >/dev/null 2>&1
[ $? -eq 0 ] && { echo "FAIL test1: expected nonzero exit"; fails=1; }
grep -qE "^PUT " "$LOG" && { echo "FAIL test1: PUT issued without password"; fails=1; }

# Tests 2-3: first full run
: > "$LOG"; : > "$BODYLOG"
ARTEX_URL="$URL" ARTEX_ADMIN_PASSWORD="pw" bash "$SCRIPT" >/dev/null 2>&1
[ $? -ne 0 ] && { echo "FAIL test2: expected exit 0"; fails=1; }
assert_agent worker hunting-methodology
assert_agent pentest hunting-methodology

# Test 4: idempotent second run -> identical PUT bodies
run1_worker="$(put_body worker)"; run1_pentest="$(put_body pentest)"
: > "$BODYLOG"
ARTEX_URL="$URL" ARTEX_ADMIN_PASSWORD="pw" bash "$SCRIPT" >/dev/null 2>&1
[ $? -ne 0 ] && { echo "FAIL test4: second run nonzero"; fails=1; }
run2_worker="$(put_body worker)"; run2_pentest="$(put_body pentest)"
[ "$run1_worker" = "$run2_worker" ] || { echo "FAIL test4: worker not idempotent"; fails=1; }
[ "$run1_pentest" = "$run2_pentest" ] || { echo "FAIL test4: pentest not idempotent"; fails=1; }

[ "$fails" -eq 0 ] && echo "ALL TESTS PASSED"
exit "$fails"
