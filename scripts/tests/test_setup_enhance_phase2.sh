#!/usr/bin/env bash
# Tests for setup-enhance-phase2.sh against a recording stub.
#  1. No admin password        -> nonzero exit, zero POSTs.
#  2. worker skills merged      -> PUT visibility preserves prior skill+mcp, adds new.
#  3. pentest skills merged     -> PUT visibility preserves prior skill+mcp, adds new.
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

# Test 1: missing password -> fail, no PUT
( unset ARTEX_ADMIN_PASSWORD; ARTEX_URL="$URL" bash "$SCRIPT" ) >/dev/null 2>&1
[ $? -eq 0 ] && { echo "FAIL test1: expected nonzero exit"; fails=1; }
grep -qE "^PUT " "$LOG" && { echo "FAIL test1: PUT issued without password"; fails=1; }

# Test 2-3: full run
: > "$LOG"; : > "$BODYLOG"
ARTEX_URL="$URL" ARTEX_ADMIN_PASSWORD="pw" bash "$SCRIPT" >/dev/null 2>&1
[ $? -ne 0 ] && { echo "FAIL test2: expected exit 0"; fails=1; }

wbody=$(grep -E "^PUT /api/agents/worker/visibility" "$BODYLOG" | head -1)
[ -z "$wbody" ] && { echo "FAIL test2: worker visibility never set"; fails=1; }
printf '%s' "$wbody" | grep -q "hunt-ssrf" || { echo "FAIL test2: hunt-ssrf not bound to worker"; fails=1; }
printf '%s' "$wbody" | grep -q "client-verify" || { echo "FAIL test2: prior worker skill not preserved"; fails=1; }
printf '%s' "$wbody" | grep -q "7" || { echo "FAIL test2: prior worker mcp not preserved"; fails=1; }

pbody=$(grep -E "^PUT /api/agents/pentest/visibility" "$BODYLOG" | head -1)
[ -z "$pbody" ] && { echo "FAIL test3: pentest visibility never set"; fails=1; }
printf '%s' "$pbody" | grep -q "hunt-cache-poison" || { echo "FAIL test3: hunt-cache-poison not bound to pentest"; fails=1; }
printf '%s' "$pbody" | grep -q "hunting-methodology" || { echo "FAIL test3: prior pentest skill not preserved"; fails=1; }

[ "$fails" -eq 0 ] && echo "ALL TESTS PASSED"
exit "$fails"
