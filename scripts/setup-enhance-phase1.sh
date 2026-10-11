#!/usr/bin/env bash
# setup-enhance-phase1.sh — idempotently wire Phase-1 enhancements over ARTEX's
# REST API: register the Playwright / writeup-search / Burp MCP servers, bind the
# skill-gated skills (client-verify / intel-dupcheck / oob-probe) to the worker
# (merged, preserving existing), and optionally pin the planner's LLM profile.
# Re-running updates in place; it never duplicates an MCP row or a binding.
#
#   ARTEX_URL             base URL (default http://127.0.0.1:8787)
#   ARTEX_ADMIN_PASSWORD  admin password for user "Penred"  (required)
#   WRITEUP_MCP_URL       writeup-search MCP endpoint (optional; skip if unset)
#   BURP_MCP_URL          burp MCP endpoint          (optional; skip if unset)
#   BURP_MCP_INSECURE     "1" to skip TLS verify for burp (optional)
#   PLANNER_PROFILE       LLM profile id or exact name to pin on planner (optional)
set -euo pipefail

ARTEX_URL="${ARTEX_URL:-http://127.0.0.1:8787}"
: "${ARTEX_ADMIN_PASSWORD:?set ARTEX_ADMIN_PASSWORD (the Penred admin password)}"

command -v curl >/dev/null || { echo "curl not found" >&2; exit 2; }
command -v jq   >/dev/null || { echo "jq not found" >&2; exit 2; }

# --- login (username is Penred post-rebrand) ---
login_resp=$(curl -fsS -X POST "$ARTEX_URL/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "$(jq -cn --arg p "$ARTEX_ADMIN_PASSWORD" '{username:"Penred",password:$p}')") \
  || { echo "login request failed against $ARTEX_URL" >&2; exit 1; }
TOKEN=$(printf '%s' "$login_resp" | jq -r '.token // empty')
[ -n "$TOKEN" ] || { echo "login failed: no token returned" >&2; exit 1; }
AUTH=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')
api() { curl -fsS "${AUTH[@]}" "$@"; }

servers_json=$(api "$ARTEX_URL/api/mcp")
mcp_exists() { printf '%s' "$servers_json" | jq -e --arg n "$1" '(.servers // .)[]? | select(.name==$n)' >/dev/null 2>&1; }

register_mcp() { # name json-payload
  if mcp_exists "$1"; then echo "mcp '$1' exists — reusing"; else
    api -X POST "$ARTEX_URL/api/mcp" -d "$2" >/dev/null && echo "registered mcp '$1'"
  fi
}

# --- playwright: stdio, self-contained in the image ---
register_mcp playwright "$(jq -cn '{name:"playwright",transport:"stdio",command:"npx",args:["@playwright/mcp"],enabled:true}')"

# --- writeup-search: http, only if URL supplied ---
if [ -n "${WRITEUP_MCP_URL:-}" ]; then
  register_mcp writeup-search "$(jq -cn --arg u "$WRITEUP_MCP_URL" '{name:"writeup-search",transport:"http",url:$u,enabled:true}')"
else echo "WRITEUP_MCP_URL unset — skipping writeup-search"; fi

# --- burp: http, only if URL supplied ---
if [ -n "${BURP_MCP_URL:-}" ]; then
  ins=false; [ "${BURP_MCP_INSECURE:-}" = "1" ] && ins=true
  register_mcp burp "$(jq -cn --arg u "$BURP_MCP_URL" --argjson i "$ins" '{name:"burp",transport:"http",url:$u,insecure:$i,enabled:true}')"
else echo "BURP_MCP_URL unset — skipping burp"; fi

# --- bind skills to the worker (merge, preserve existing skill + mcp) ---
NEW_SKILLS='["client-verify","intel-dupcheck","oob-probe"]'
cur=$(api "$ARTEX_URL/api/agents/worker/visibility")
merged=$(printf '%s' "$cur" | jq -c --argjson add "$NEW_SKILLS" '{mcp:(.mcp//[]),skill:((.skill//[])+$add|unique)}')
api -X PUT "$ARTEX_URL/api/agents/worker/visibility" -d "$merged" >/dev/null
echo "bound skills to worker (merged)"

# --- optional: pin the planner LLM profile ---
if [ -n "${PLANNER_PROFILE:-}" ]; then
  profiles=$(api "$ARTEX_URL/api/llm")
  pid=$(printf '%s' "$profiles" | jq -r --arg p "$PLANNER_PROFILE" \
    '((.profiles // .)[]? | select((.id|tostring)==$p or .name==$p) | .id) // empty' | head -1)
  if [ -n "$pid" ]; then
    api -X PUT "$ARTEX_URL/api/agents/planner" -d "$(jq -cn --argjson id "$pid" '{llm_profile_id:$id}')" >/dev/null
    echo "pinned planner LLM profile -> $pid"
  else echo "PLANNER_PROFILE '$PLANNER_PROFILE' not found — leaving planner untouched"; fi
else echo "PLANNER_PROFILE unset — leaving planner untouched"; fi

echo "Phase-1 enhancement seed complete."
