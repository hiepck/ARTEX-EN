#!/usr/bin/env bash
# Tests for setup-enhance-phase1.sh against a recording stub.
#  1. No admin password        -> nonzero exit, zero POSTs.
#  2. playwright exists         -> exit 0, no duplicate POST /api/mcp for it.
#  3. worker skills merged      -> PUT visibility preserves prior skill+mcp, adds new.
#  4. optional URLs unset       -> no writeup-search/burp mcp POST; others proceed.
#  5. PLANNER_PROFILE unset     -> no PUT /api/agents/planner (binding untouched).
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/../setup-enhance-phase1.sh"
STUB="$HERE/stub_enhance_phase1.py"
PORT=$(( (RANDOM % 20000) + 20000 ))
URL="http://127.0.0.1:$PORT"
LOG="$(mktemp)"; BODYLOG="$(mktemp)"; fails=0
export STUB_LOG="$LOG" STUB_BODYLOG="$BODYLOG"
python3 -I "$STUB" "$PORT" & STUB_PID=$!
disown "$STUB_PID" 2>/dev/null || true
cleanup() { kill "$STUB_PID" 2>/dev/null; rm -f "$LOG" "$BODYLOG"; }
trap cleanup EXIT
for _ in $(seq 1 50); do curl -fsS "$URL/api/mcp" >/dev/null 2>&1 && break; sleep 0.1; done
: > "$LOG"

# Test 1: missing password -> fail, no POST
( unset ARTEX_ADMIN_PASSWORD; ARTEX_URL="$URL" bash "$SCRIPT" ) >/dev/null 2>&1
[ $? -eq 0 ] && { echo "FAIL test1: expected nonzero exit"; fails=1; }
grep -q "^POST " "$LOG" && { echo "FAIL test1: POST issued without password"; fails=1; }

# Test 2-5: full run, playwright-only (no optional URLs, no planner profile)
: > "$LOG"; : > "$BODYLOG"
ARTEX_URL="$URL" ARTEX_ADMIN_PASSWORD="pw" bash "$SCRIPT" >/dev/null 2>&1
[ $? -ne 0 ] && { echo "FAIL test2: expected exit 0"; fails=1; }
if grep -qiE '"name":"?playwright' "$BODYLOG" && grep -qE "^POST /api/mcp$" "$LOG"; then
  echo "FAIL test2: duplicate playwright mcp POST (not idempotent)"; fails=1; fi
wbody=$(grep -E "^PUT /api/agents/worker/visibility" "$BODYLOG" | head -1)
[ -z "$wbody" ] && { echo "FAIL test3: worker visibility never set"; fails=1; }
printf '%s' "$wbody" | grep -q "client-verify" || { echo "FAIL test3: new skill not bound"; fails=1; }
printf '%s' "$wbody" | grep -q "api-recon" || { echo "FAIL test3: prior skill not preserved"; fails=1; }
printf '%s' "$wbody" | grep -q "7" || { echo "FAIL test3: prior mcp not preserved"; fails=1; }
grep -qi "writeup-search" "$BODYLOG" && { echo "FAIL test4: writeup mcp registered without URL"; fails=1; }
grep -qiE '"name":"?burp' "$BODYLOG" && { echo "FAIL test4: burp mcp registered without URL"; fails=1; }
grep -qE "^PUT /api/agents/planner$" "$LOG" && { echo "FAIL test5: planner profile set without PLANNER_PROFILE"; fails=1; }

[ "$fails" -eq 0 ] && echo "ALL TESTS PASSED"
exit "$fails"
