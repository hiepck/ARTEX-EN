#!/usr/bin/env bash
# setup-enhance-phase2.sh — idempotently bind Phase-2 exotic-class skills to the
# worker and pentest agents. Re-running preserves existing bindings.
#
#   ARTEX_URL             base URL (default http://127.0.0.1:8787)
#   ARTEX_ADMIN_PASSWORD  admin password for user "admin" (required)
set -euo pipefail

ARTEX_URL="${ARTEX_URL:-http://127.0.0.1:8787}"
: "${ARTEX_ADMIN_PASSWORD:?set ARTEX_ADMIN_PASSWORD (the admin password)}"

command -v curl >/dev/null || { echo "curl not found" >&2; exit 2; }
command -v jq   >/dev/null || { echo "jq not found" >&2; exit 2; }

# --- login ---
login_resp=$(curl -fsS -X POST "$ARTEX_URL/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg p "$ARTEX_ADMIN_PASSWORD" '{username:"admin",password:$p}')") \
  || { echo "login request failed against $ARTEX_URL" >&2; exit 1; }
TOKEN=$(printf '%s' "$login_resp" | jq -r '.token // empty')
[ -n "$TOKEN" ] || { echo "login failed: no token returned" >&2; exit 1; }
AUTH=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')
api() { curl -fsS "${AUTH[@]}" "$@"; }

# --- skill lists ---
WORKER_SKILLS='["hunt-ssrf","hunt-cache-poison","hunt-race-conditions","hunt-graphql-authz","hunt-smuggling"]'
PENTEST_SKILLS='["hunt-ssrf","hunt-cache-poison","hunt-race-conditions","hunt-graphql-authz","hunt-smuggling"]'

merge_visibility() {
  local agent="$1"
  local add="$2"
  local cur
  cur=$(api "$ARTEX_URL/api/agents/$agent/visibility")
  printf '%s' "$cur" | jq -c --argjson add "$add" '{mcp:(.mcp//[]),skill:((.skill//[])+$add|unique)}'
}

worker_merged=$(merge_visibility worker "$WORKER_SKILLS")
api -X PUT "$ARTEX_URL/api/agents/worker/visibility" -d "$worker_merged" >/dev/null
echo "bound Phase-2 skills to worker (merged)"

pentest_merged=$(merge_visibility pentest "$PENTEST_SKILLS")
api -X PUT "$ARTEX_URL/api/agents/pentest/visibility" -d "$pentest_merged" >/dev/null
echo "bound Phase-2 skills to pentest (merged)"

echo "Phase-2 skill binding seed complete."
