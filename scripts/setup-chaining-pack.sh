#!/usr/bin/env bash
# setup-chaining-pack.sh — idempotently create and wire the "chainer" custom agent
# over ARTEX's REST API: prompt, skill bindings, tool bindings, and an on_finding
# trigger. Re-running updates in place; it never creates a second agent or trigger.
#
# Requirements: curl, jq, and a running ARTEX with its admin password set.
#   ARTEX_URL               base URL (default http://127.0.0.1:8787)
#   ARTEX_ADMIN_PASSWORD    the admin ("ARTEX") password  (required)
set -euo pipefail

ARTEX_URL="${ARTEX_URL:-http://127.0.0.1:8787}"
: "${ARTEX_ADMIN_PASSWORD:?set ARTEX_ADMIN_PASSWORD (the ARTEX admin password)}"

AGENT_KEY="chainer"
SKILLS='["chain-hunting","chain-toolkit","validation-gate"]'
# Hunting skills deepen the worker's per-class bug finding (spec goal #1).
WORKER_SKILLS='["hunting-methodology","hunt-idor","hunt-oauth","hunt-xss","hunt-rce","hunt-business-logic","hunt-info-disclosure","validation-gate"]'
# Read/correlate/plan tools for the chainer — plus report_finding for the composite only.
TOOLS=(list_findings list_task_findings get_task_node_detail get_task_graph \
       list_task_worker_traces get_task_worker_trace add_task_hint record_fact Bash report_finding)

command -v curl >/dev/null || { echo "curl not found" >&2; exit 2; }
command -v jq   >/dev/null || { echo "jq not found" >&2; exit 2; }

# --- login ---
login_resp=$(curl -fsS -X POST "$ARTEX_URL/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg p "$ARTEX_ADMIN_PASSWORD" '{username:"Penred",password:$p}')") \
  || { echo "login request failed against $ARTEX_URL" >&2; exit 1; }
TOKEN=$(printf '%s' "$login_resp" | jq -r '.token // empty')
[ -n "$TOKEN" ] || { echo "login failed: no token returned" >&2; exit 1; }

AUTH=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')
api() { curl -fsS "${AUTH[@]}" "$@"; }

# --- create agent if absent (idempotent) ---
if api "$ARTEX_URL/api/agents" | jq -e --arg k "$AGENT_KEY" '.agents[]? | select(.key==$k)' >/dev/null 2>&1; then
  echo "agent '$AGENT_KEY' already exists — reusing"
else
  api -X POST "$ARTEX_URL/api/agents" \
    -d "$(jq -n --arg k "$AGENT_KEY" '{Key:$k,Name:"Chainer",Description:"Correlates findings and plans exploit chains on each new finding."}')" >/dev/null
  echo "created agent '$AGENT_KEY'"
fi

# --- prompt (idempotent PUT) ---
read -r -d '' PROMPT <<'EOF' || true
You are the exploit-chain analysis agent inside an authorized penetration-testing system.
You do not drive traffic at the target yourself. On each new confirmed finding you:
1. Read the current findings and the exploration graph.
2. Rank next-hop candidates (use the chain-toolkit chain_plan.py helper and chain-hunting/references/chain-table.md).
3. For an IN-SCOPE candidate, plant a chain-pursuit hint with add_task_hint: state the proven capability, the specific next hop, and the terminal impact it targets. The planner dispatches a worker to run the hop through the normal scope-gated path.
4. Enforce the per-hop evidence gate: a hop counts only when its postcondition is independently observed; chain confidence is the minimum across hops; any theoretical hop makes the chain non-reportable.
5. Re-check scope on every hop. An out-of-scope hop is a hard stop — never pursue it.
6. When a chain reaches a terminal impact with every hop proven, score it with cvss_calc.py and register the composite with report_finding (vulnclass="chain"), referencing the leg finding IDs.
Follow the chain-hunting and validation-gate skills. Register leg findings via workers, not yourself.
EOF
api -X PUT "$ARTEX_URL/api/agents/$AGENT_KEY/prompt" \
  -d "$(jq -n --arg t "$PROMPT" '{Template:$t,Note:"chaining pack seed"}')" >/dev/null
echo "prompt set"

# --- skill visibility (idempotent PUT) ---
api -X PUT "$ARTEX_URL/api/agents/$AGENT_KEY/visibility" \
  -d "$(jq -n --argjson s "$SKILLS" '{mcp:[],skill:$s}')" >/dev/null
echo "bound chainer skills: $SKILLS"

# --- bind hunting skills to the worker, MERGING with its existing skills + mcp ---
# (PUT overwrites visibility, so read-modify-write to avoid dropping prior bindings.)
wvis=$(api "$ARTEX_URL/api/agents/worker/visibility")
merged=$(printf '%s' "$wvis" | jq -c --argjson add "$WORKER_SKILLS" \
  '{mcp:(.mcp // []), skill:((.skill // []) + $add | unique)}')
api -X PUT "$ARTEX_URL/api/agents/worker/visibility" -d "$merged" >/dev/null
echo "bound hunting skills to worker (merged with existing)"

# --- tool bindings (read current row, append chainer if absent, PUT) ---
tools_json=$(api "$ARTEX_URL/api/tools")
for key in "${TOOLS[@]}"; do
  row=$(printf '%s' "$tools_json" | jq -c --arg k "$key" '.tools[]? | select(.key==$k)')
  if [ -z "$row" ]; then
    echo "  tool '$key' not found — skipping" >&2
    continue
  fi
  if printf '%s' "$row" | jq -e --arg a "$AGENT_KEY" '(.agents // []) | index($a)' >/dev/null 2>&1; then
    continue  # already bound
  fi
  body=$(printf '%s' "$row" | jq -c --arg a "$AGENT_KEY" \
    '{description:(.description // ""), schema:(.schema // {}), agents:((.agents // []) + [$a]), enabled:(.enabled // true)}')
  api -X PUT "$ARTEX_URL/api/tools/$key" -d "$body" >/dev/null
  echo "  bound tool '$key'"
done

# --- on_finding trigger (create only if none) ---
if api "$ARTEX_URL/api/agents/$AGENT_KEY/triggers" | jq -e '.triggers[]? | select(.on_finding==true)' >/dev/null 2>&1; then
  echo "on_finding trigger already present — leaving it"
else
  api -X POST "$ARTEX_URL/api/agents/$AGENT_KEY/triggers" \
    -d "$(jq -n '{enabled:true,on_finding:true,finding_message:"A new finding was just confirmed. Correlate it with existing findings, rank next-hop candidates, and (for in-scope hops) plant a chain-pursuit hint toward a terminal impact."}')" >/dev/null
  echo "created on_finding trigger"
fi

echo "chaining pack setup complete."
