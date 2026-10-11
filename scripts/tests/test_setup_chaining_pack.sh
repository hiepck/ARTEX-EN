#!/usr/bin/env bash
# Tests for setup-chaining-pack.sh against a recording stub.
#  1. No admin password  -> nonzero exit, zero requests.
#  2. Agent + trigger already exist -> exit 0, no create POSTs (idempotent).
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$HERE/../setup-chaining-pack.sh"
STUB="$HERE/stub_artex.py"
PORT=$(( (RANDOM % 20000) + 20000 ))
URL="http://127.0.0.1:$PORT"
LOG="$(mktemp)"
BODYLOG="$(mktemp)"
fails=0

export STUB_LOG="$LOG"
export STUB_BODYLOG="$BODYLOG"
python3 -I "$STUB" "$PORT" &
STUB_PID=$!
disown "$STUB_PID" 2>/dev/null || true
cleanup() { kill "$STUB_PID" 2>/dev/null; rm -f "$LOG" "$BODYLOG"; }
trap cleanup EXIT

# wait for the stub to accept connections
for _ in $(seq 1 50); do
  curl -fsS "$URL/api/tools" >/dev/null 2>&1 && break
  sleep 0.1
done
: > "$LOG"  # discard the readiness probe

# --- Test 1: missing password -> fail fast, no requests ---
( unset ARTEX_ADMIN_PASSWORD; ARTEX_URL="$URL" bash "$SCRIPT" ) >/dev/null 2>&1
rc=$?
if [ "$rc" -eq 0 ]; then echo "FAIL test1: expected nonzero exit without password"; fails=1; fi
if grep -q "^POST " "$LOG"; then echo "FAIL test1: a POST was issued without a password"; fails=1; fi

# --- Test 2: existing agent + trigger -> exit 0, no create POSTs ---
: > "$LOG"; : > "$BODYLOG"
ARTEX_URL="$URL" ARTEX_ADMIN_PASSWORD="pw" bash "$SCRIPT" >/dev/null 2>&1
rc=$?
if [ "$rc" -ne 0 ]; then echo "FAIL test2: expected exit 0 on idempotent run (got $rc)"; fails=1; fi
if grep -qE "^POST /api/agents$" "$LOG"; then echo "FAIL test2: created a duplicate agent"; fails=1; fi
if grep -qE "^POST /api/agents/chainer/triggers$" "$LOG"; then echo "FAIL test2: created a duplicate trigger"; fails=1; fi

# --- Test 3: worker hunting skills bound, merged (preserve existing skill + mcp) ---
if ! grep -qE "^PUT /api/agents/worker/visibility" "$LOG"; then echo "FAIL test3: worker visibility never set"; fails=1; fi
wbody=$(grep -E "^PUT /api/agents/worker/visibility" "$BODYLOG" | head -1)
if ! printf '%s' "$wbody" | grep -q "hunting-methodology"; then echo "FAIL test3: hunting-methodology not bound to worker"; fails=1; fi
if ! printf '%s' "$wbody" | grep -q "api-recon"; then echo "FAIL test3: pre-existing worker skill (api-recon) not preserved"; fails=1; fi
if ! printf '%s' "$wbody" | grep -q "7"; then echo "FAIL test3: pre-existing worker mcp (7) not preserved"; fails=1; fi

if [ "$fails" -eq 0 ]; then echo "OK: all setup-chaining-pack tests passed"; fi
exit "$fails"
