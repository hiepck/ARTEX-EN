package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// Planner is the event-driven LLM planner (docs §4.3): each time the asset or
// exploration graph changes (debounced), it reads the exploration route, queries
// assets, judges whether the task goal is met, and emits 0..N exploration intents
// into the frontier. It is the sole intent generator.
type Planner struct {
	findingRecorder   FindingRecorder
	prov              llm.Provider
	model             string
	tx                *transcript.Store                      // raw LLM conversation persistence (nil = off)
	window            int                                    // context window in tokens (for compaction)
	windowFn          func() int                             // optional dynamic task-chain minimum
	maxTurns          int                                    // max agent turns per run (0 = unlimited)
	killWork          func(intentID int64) error             // engine callback to terminate a running work (nil = off)
	steerWork         func(intentID int64, msg string) error // engine callback to steer a running work mid-run (nil = off)
	proxyAddr         string                                 // recording proxy for WebFetch (empty = direct)
	proxyCACert       string                                 // recording proxy's CA cert path (HTTPS verify)
	webSearch         WebSearchOpts                          // web_search tool backend selection (off by default)
	workDir           string                                 // shared work dir (surfaced in prompt as artifact-output target)
	injectConstraints func() bool                            // resolver: inject task operation constraints into system prompt? (nil = yes)
	nonStreamingFn    func() bool                            // resolver: use non-streaming (Complete) path? (nil = streaming)
	noaEnabledFn      func() bool                            // resolver: use experimental noa compaction? (nil = off)
	maxTokensFn       func() int                             // resolver: per-reply output cap (nil/0 = send no cap)
	compactor         *Compactor                             // cold-node compaction (§7); nil = disabled

	// todos keeps ONE plan-scratchpad per task (keyed by exploration id) so the
	// planner's multi-step plan survives across wake-ups — each Plan() is a fresh
	// session, but the shared store lets it record a serial exploit chain once and
	// dispatch it step-by-step over rounds instead of front-loading it in parallel.
	todoMu sync.Mutex
	todos  map[int64]*actool.TodoStore
}

func NewPlanner(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *Planner {
	return &Planner{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns, todos: map[int64]*actool.TodoStore{}}
}

func (p *Planner) SetCompactionWindowResolver(fn func() int) { p.windowFn = fn }

// SetCompactor wires the cold-node compactor (cold-digest §7). Called each
// planner wake-up to advance the round counter, maintain cold stamps, and
// (off the hot path) fold cold nodes into digests. nil = feature disabled.
func (p *Planner) SetCompactor(c *Compactor) { p.compactor = c }

// SetNonStreaming wires a resolver deciding whether runs use the non-streaming
// model path (true = non-streaming). nil/unset = streaming (default).
func (p *Planner) SetNonStreaming(fn func() bool) { p.nonStreamingFn = fn }

func (p *Planner) nonStreaming() bool { return p.nonStreamingFn != nil && p.nonStreamingFn() }

// SetNoaEnabled wires a resolver deciding whether runs use the experimental noa
// context-compression mechanism. nil/unset = off (built-in compaction). Read per
// run so the settings toggle takes effect without rebuilding the agent.
func (p *Planner) SetNoaEnabled(fn func() bool) { p.noaEnabledFn = fn }

// SetMaxTokens wires a resolver for the per-reply output cap. nil/unset or 0 =
// send no cap and let the endpoint decide. Read per run, like nonStreaming.
func (p *Planner) SetMaxTokens(fn func() int) { p.maxTokensFn = fn }

func (p *Planner) maxTokens() int {
	if p.maxTokensFn == nil {
		return 0
	}
	return p.maxTokensFn()
}

func (p *Planner) compactionWindow() int {
	if p.windowFn != nil {
		return p.windowFn()
	}
	return p.window
}

// SetProxy points the planner's WebFetch at the recording proxy plus the CA cert
// it trusts to verify HTTPS through it (empty addr = direct).
func (p *Planner) SetProxy(addr, caCert string) { p.proxyAddr, p.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the planner (off by default).
func (p *Planner) SetWebSearch(o WebSearchOpts) { p.webSearch = o }

// SetConstraintInject wires a resolver deciding whether this task's operation
// constraints get injected into the planner system prompt. Read per round so the
// settings toggle takes effect without rebuilding the agent. nil = inject (default).
func (p *Planner) SetConstraintInject(fn func() bool) { p.injectConstraints = fn }

// wantConstraints reports whether constraint injection is enabled (default yes).
func (p *Planner) wantConstraints() bool { return p.injectConstraints == nil || p.injectConstraints() }

// todoFor returns the task's persistent planning todo store, creating it on first
// use. Shared across all of this task's planner wake-ups.
func (p *Planner) todoFor(expID int64) *actool.TodoStore {
	p.todoMu.Lock()
	defer p.todoMu.Unlock()
	s := p.todos[expID]
	if s == nil {
		s = actool.NewTodoStore()
		p.todos[expID] = s
	}
	return s
}

// SetKillWork wires the engine's per-work terminate callback so the planner's
// kill_work tool can stop a single running worker.
func (p *Planner) SetKillWork(fn func(intentID int64) error) { p.killWork = fn }

// SetSteerWork wires the engine's per-work steering callback so the planner's
// steer_work tool can inject a mid-run course-correction into a running worker.
func (p *Planner) SetSteerWork(fn func(intentID int64, msg string) error) { p.steerWork = fn }

// renderPlannerTodos formats the persistent planning todo for injection into the
// wake-up prompt (empty when there are no todos yet — first wake-up).
func renderPlannerTodos(items []actool.Todo) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[Your planning todo list (retained across wake-ups, written by you last round)]:\n")
	for _, it := range items {
		mark := map[actool.TodoStatus]string{actool.TodoPending: "☐", actool.TodoInProgress: "▶", actool.TodoCompleted: "✔"}[it.Status]
		if mark == "" {
			mark = "☐"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, it.Content))
	}
	b.WriteString("Use it to make progress: dispatch an intent only for the next step whose [prerequisite steps are complete / whose required fact already exists]; update the list with TodoWrite (mark steps satisfied by a fact as completed). Do not re-dispatch a step already pending/in_progress on the list.")
	return b.String()
}

// TriggerEvent describes what concretely caused this planning round to fire, so
// the planner looks first at the actual change instead of re-scanning the whole
// overview. Kind:
//
//	"done"    — a worker finished intent IntentID (its output conclusion is fetched).
//	"finding" — a worker reported a finding on intent IntentID (Detail = the summary).
//	"goal"    — the human (through the main agent's set_goals) added one OR MORE goals in a
//	            single call (Goals = the goal texts added this time, 1+; set_goals supports batching).
//	"goal_deleted" — the human deleted a goal from the overview's goal management (Detail = the deleted goal text).
//	"goal_edited"  — the human edited a goal from the overview's goal management (OldGoal -> NewGoal text).
//	"cancelled" — the human deleted intent IntentID (Detail = the deletion reason). The intent is
//	            stopped (not deleted) and the reason is attached to it as a fact.
type TriggerEvent struct {
	Kind     string
	IntentID int64
	Detail   string
	Summary  string   // Kind=="cancelled" only: the intent summary captured before deletion (the node no longer exists after a hard delete and cannot be looked up)
	Goals    []string // Kind=="goal" only: the goal texts added by this set_goals call (one or more)
	OldGoal  string   // Kind=="goal_edited" only: the goal text before the edit
	NewGoal  string   // Kind=="goal_edited" only: the goal text after the edit
	Hints    []string // Kind=="hint" only: the hint texts added by this add_hint call (one or more)
}

// renderTriggers spells out the change(s) that fired this round: for a finished
// worker — which intent + its output conclusion; for a finding — which intent +
// what was found. Empty for time/heartbeat wakes. Reads the store (best-effort;
// a blank field never blocks the round).
func renderTriggers(ts *db.ExplorationStore, evs []TriggerEvent) string {
	if len(evs) == 0 || ts == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n[What actually changed to trigger this round (read this first, then decide whether to add directions)]:")
	for _, ev := range evs {
		switch ev.Kind {
		case "goal":
			if len(ev.Goals) == 1 {
				b.WriteString(fmt.Sprintf("\n- The human (main agent) added a goal: %s -- a new goal to achieve; add an exploration direction for it (if no intent covers it yet).", ev.Goals[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- The human (main agent) added %d goals: %s -- all are new goals to achieve; add an exploration direction for each that no intent covers yet.", len(ev.Goals), strings.Join(ev.Goals, "; ")))
			}
		case "hint":
			if len(ev.Hints) == 1 {
				b.WriteString(fmt.Sprintf("\n- The human (main agent) added a strategic hint: %s -- it is attached to the exploration graph; adjust or add exploration directions accordingly (if no intent covers it yet).", ev.Hints[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- The human (main agent) added %d strategic hints: %s -- all are attached to the exploration graph; adjust or add exploration directions for each of them.", len(ev.Hints), strings.Join(ev.Hints, "; ")))
			}
		case "goal_deleted":
			b.WriteString(fmt.Sprintf("\n- The human deleted this goal: %s -- it has been removed, so re-judge the remaining goals/directions accordingly (no need to dispatch intents for it any more).", ev.Detail))
		case "goal_edited":
			b.WriteString(fmt.Sprintf("\n- The human edited a goal, from %q to %q -- adjust the exploration directions to the new goal (stop dispatching the old direction if it no longer applies).", ev.OldGoal, ev.NewGoal))
		case "finding":
			b.WriteString(fmt.Sprintf("\n- The worker of intent #%d (%s) reported a finding: %s", ev.IntentID, intentSummary(ts, ev.IntentID), ev.Detail))
		case "cancelled":
			// Prefer the Summary captured at deletion time (the node no longer exists after a hard delete, so intentSummary cannot find it).
			sm := ev.Summary
			if sm == "" {
				sm = intentSummary(ts, ev.IntentID)
			}
			b.WriteString(fmt.Sprintf("\n- Intent #%d was deleted by the user; its content was: %s, and the reason was: %s. That intent is deleted (it will not run); re-plan accordingly.", ev.IntentID, sm, ev.Detail))
		default: // "done"
			b.WriteString(fmt.Sprintf("\n- The worker of intent #%d (%s) finished with the conclusion: %s", ev.IntentID, intentSummary(ts, ev.IntentID), workerOutput(ts, ev.IntentID)))
			if fids := factIDsYielded(ts, ev.IntentID); fids != "" {
				b.WriteString(fmt.Sprintf("; the fact ids newly produced by this intent: %s ", fids))
			}
		}
	}
	b.WriteString("\n(Full details can be looked up with node_detail / get_worker_output / list_findings.)")
	return b.String()
}

// factIDsYielded lists the fact ids an intent produced this run as "#12, #15", so the
// planner can jump straight to the round's incremental facts. Empty (best-effort) when
// the intent yielded no facts or the lookup fails.
func factIDsYielded(ts *db.ExplorationStore, id int64) string {
	ids, err := ts.FactsYielded(id)
	if err != nil || len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, fid := range ids {
		parts[i] = fmt.Sprintf("#%d", fid)
	}
	return strings.Join(parts, ", ")
}

// intentSummary reads an intent node's one-line summary (best-effort, "?" on miss).
func intentSummary(ts *db.ExplorationStore, id int64) string {
	n, err := ts.GetNode(id)
	if err != nil || n == nil {
		return "?"
	}
	var p map[string]any
	if json.Unmarshal(n.Payload, &p) == nil {
		if s, ok := p["summary"].(string); ok && s != "" {
			return s
		}
	}
	return "?"
}

// workerOutput returns the finished worker's conclusion for an intent — the last
// 'result' (else 'text') activity's full detail, truncated. Same source get_worker_output uses.
func workerOutput(ts *db.ExplorationStore, id int64) string {
	acts, _, err := ts.ActivityList(&id, 0, 1000)
	if err != nil {
		return "(failed to fetch the output)"
	}
	var pick *db.Activity
	for i := range acts {
		if acts[i].Kind == "result" {
			pick = &acts[i]
		} else if acts[i].Kind == "text" && pick == nil {
			pick = &acts[i]
		}
	}
	if pick == nil {
		return "(this work has no recorded output)"
	}
	out, _ := ts.ActivityDetail(pick.ID)
	if out == "" {
		out = pick.Summary
	}
	return truncOutput(out, 800)
}

// truncOutput caps a worker-output blob so the trigger context doesn't bloat the
// system prompt every round; full text is one get_worker_output call away.
func truncOutput(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " ... (truncated; see get_worker_output for the full text)"
}

// renderGraphOverview folds the pre-computed graph_overview snapshot into the
// wake-up prompt so the planner starts each round with the full situation in
// hand — saving the round-trip it would otherwise spend calling the tool. It is
// the exact same JSON graph_overview would return; deeper detail is still one
// tool call away (node_detail / list_facts / …).
func renderGraphOverview(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "" // fall back to the model calling graph_overview itself
	}
	return "\n\n[This round's state (prefetched graph_overview, identical to what that tool would return; call node_detail/list_facts etc. when you need details)]:\n" + string(b)
}

// plannerDefaultTmpl is the built-in EDITABLE body (section [A]) of the planner prompt,
// seeded into agent_prompts. Goal is a {{.Goal}} template var; the intermediate artifact output spec
// tail is code-owned (artifactSpec) and appended by plannerSystem after rendering.
const plannerDefaultTmpl = `You are the "planner" of an authorized penetration testing system on a security platform, woken frequently (any change to the graph wakes you). Your job: read the state -> judge the goals -> **add exploration intents only when there really is an uncovered new direction**. You are the planner, not the executor: everything you produce this round can only be [generating/clarifying intents] or [judging goals], and you never do the work yourself inside a plan.

Task goal: {{.Goal}}

**How many intents should this round produce (think this through first)**:
- **Hard floor (highest priority)**: whenever [the goal is not achieved] and [there is no open or running intent at all] (frontier_open=0 and running_intents empty), this round [must] produce at least one intent that advances the goal -- with no running work to wait for and no queued direction, producing 0 intents means the task stalls; even when every known direction appears only in recent_done, open another one or continue one based on the done/exhausted/blocked judgement below.
- Beyond that hard floor, **producing 0 intents is a normal outcome but needs a legitimate reason** (not a default of "dispatching less is safer"): (1) **already covered** -- every direction you thought of is handled by an intent still open/running (regenerating an existing intent in different words is a serious error); (2) **waiting on a dependency** -- the next step depends on the output of running work that has not arrived yet (forcing a dispatch now would leave downstream work spinning without its prerequisite, so wait for the next wake-up after the graph updates).
- Conversely: when there really is a new direction that is [uncovered and does not depend on running work], or the goal is unachieved and untested surface remains in scope, dispatch it -- do not treat 0 intents as a lazy default.

**The decision procedure for each wake-up**:

1. **The full state is attached below this prompt** (it is graph_overview's return value, so there is no need to call it again): task (the original title + goal/root node), asset counts, goals + their states, open/running/recent_done intents, sites_without_endpoints (sites with no endpoints, hinting at directions still to explore), facts (the number of exploration facts, a different category from findings), recent_facts ({id,summary,confidence?}), coverage ({pct, by_type:[{type,total,tested,untested}]} — an asset type with untested>0 is the clearest coverage gap; do not treat pct alone as "done") and findings_by_vulnclass (how many findings exist per vulnerability class — a class absent there is not proof it was tried, but a class present is a proven working PoC worth extending).
   - **Scope**: exploration nodes (goals/intents/facts/findings) cover this task only; **the asset graph is globally shared** (one copy across tasks, so the asset count is global within scope and not unique to this task) -- ignore assets unrelated to this task when they appear.
   - **Lineage**: every intent carries parents (upstream: which facts/intents it derives from) and yields (downstream: which facts/findings it produced), and each recent_facts entry carries from_intent; use them to understand "which facts came from which direction, and whether they combine into a new one".
   - **Negative/uncertain observations** (such as "the port is closed" / "not injectable" in recent_facts) are a worker's observations, not verdicts: before accepting one, read its evidence with node_detail(id) -- only solid evidence with confidence=observed and exhausted techniques counts as that direction being closed for now; when evidence is missing, it is merely "it looks like it / probed once", or confidence=inferred, treat it as [not yet determined] and, if it is in scope and no other intent covers it, dispatch a re-check intent by default to confirm or overturn it (**at most one re-check per negative direction**; if it is still negative after the re-check and the evidence is reasonable, respect that conclusion and stop dispatching).
   - **Call for deeper detail only when needed**: list_facts (paginated, newest first, default 20, filterable with q, pageable with before, carrying total/has_more), list_findings (all findings), node_detail(id) (the full evidence/details; lists and recent_facts give summaries only), list_assets (pull: q search, type/company_id/task_id filters, paginated, or fetch directly by id/ids), asset_neighbors. The asset graph is globally shared, so do not pull all of it by default.

2. **Judge the goals (your core job)**: the goals field already carries the goals and their states; for an unachieved goal that some finding/fact has proved, call prove_goal(goal_id, evidence_id, reason) to mark it met. **When the one you mark happens to be the last unfinished goal, the system judges the whole task complete automatically** -- wrapping up is driven only by proving goals one by one, and there is no other "finish it all" mechanism.
   - ! **Quantitative acceptance check (never rubber-stamp early)**: when a goal carries a quantifiable condition (coverage reaches X%, capture N flags, obtain some privilege), you [must] check the measured values in graph_overview above (coverage.pct, the findings_total count etc.) before prove_goal: if it is not met, prove_goal is [forbidden] and you dispatch intents to close the gap instead; marking it met early on the grounds of "mostly achieved / the core is taken" is not allowed. Example: the requirement is 100% coverage but the measurement is coverage.pct=40% -> not achieved, keep dispatching intents to test more.

3. **(Optional, opening only, extremely light) probing to understand**: only when the graph has almost no facts yet (recent_facts essentially empty, the task has just started) and the state alone cannot make the initial intent concrete, use Bash or similar for a very small number of read-only probes of the target (such as 1-2 curls at the homepage/fingerprint). **The only legitimate product is a more precise intent description** -- never the discovery/verification/exploitation of a vulnerability, and never an enumeration of endpoints/directories/parameters (that is the worker's work; write it as an intent and dispatch it). Three hard boundaries:
   - The graph already has worker-produced facts (facts>0 / recent_facts non-empty) -> probing yourself is [forbidden]; base every judgement on the existing facts, and this round's output can only be "dispatch new intents" or "finish". To dig into a lead, dispatch an intent for a worker to do it rather than curling yourself.
   - Even at the opening, probe at most 3 times and then stop, purely to state the initial intent clearly; the moment you notice you are "investigating in depth" rather than "quickly fixing a direction" (enumerating endpoints/directories one by one, trying ids one by one, decoding chains, probing the same endpoint repeatedly, any injection/authorization/vulnerability testing -- all the worker's heavy lifting), stop immediately and write it as an intent.
   - When existing facts and state are enough to judge, no probing is needed at all.

4. **Decide which new directions to add**: **"restraint" here means only [do not duplicate an existing intent], not "dispatch as few as possible"** -- while the goal is unachieved, the default question is "to close in on the goal, what deeper and harder plays remain uncovered", not "can we wrap up". An intent is an [open exploration direction] (not a fixed type or menu); judge directions yourself from the known facts, assets and goal, and compare each against open + running + recent_done:
   - Already covered by an open/running intent -> do not generate it (it is being handled).
   - It appeared in recent_done -> **first look at that intent's state (every entry carries it) to see how it stopped, then decide**:
     - **done (ran to completion)**: already covered -> do not re-dispatch it as-is; whether it is a dead end follows from the facts it yielded, not from the state; re-dispatch only when [materially new mechanics] appear (a new fact/asset/parameter/clearly different play), and say in the summary how it differs from last time; rewording it or "maybe it will work if I try again" does not count, and retrying is forbidden.
     - **exhausted (budget spent, cut off midway with only part written back) / blocked (a model or network failure, essentially nothing explored)**: both ended badly with incomplete information -- first use get_worker_trace / get_worker_output to see how far it actually got and where it stuck, then choose from: close to a breakthrough but cut off by budget -> dispatch "continue from last time's progress"; a purely external failure that never ran (often the case for blocked) -> re-dispatch the same direction; stuck at the same point every time -> change the play or the direction. The basis is always the real progress in the trace, never the state itself.
   - A brand new direction covered by no intent at all -> generate it.
   - Every known direction is covered by an intent still open/running -> generate nothing and simply finish (there is running/queued work, let it make progress); but if only recent_done covers them, nothing is open/running, and the goal is unachieved -> the hard floor at the top requires opening another or continuing one.
   - **Depth beats coverage**: coverage is a floor / an acceptance criterion, not the exploration goal itself; once a high-value entry point is found (one that may lead to RCE/privilege escalation/data exfiltration), prioritize dispatching intents to [drive that path all the way through] rather than spreading shallow tests across assets to raise the coverage number.
   - **Keep the routes diverse, do not converge too early**: while the goal is unachieved, if every existing intent is crowded onto the same route/entry point while a [fundamentally different] uncovered direction exists (another entry surface / another asset class / another exploitation chain), add that divergent direction first rather than piling synonymous intents onto the same line (judge by substance, not wording); if that divergent direction is already covered by an existing intent, still generate nothing. Ideally 2-3 mechanically different routes coexist (such as "attack through the upload chain" and "attack through an authentication bypass"), and resources concentrate on one only after it produces evidence of [closing in on the goal]. **But diversity always yields to the [operating constraints] at the top**: an entry surface/port/host/operation excluded by a constraint never gets an intent, however different it is.
   - **Watch burn on running intents (when visible)**: when an admin has turned the setting on, each open/running intent above carries a burn figure (tool calls / tokens / elapsed seconds since it was claimed). A running intent with a high burn and no new fact/finding to show for it is a candidate for action now, rather than waiting out its max_turns/run_seconds cap: look at it with get_worker_output first, then steer_work for a correctable direction or kill_work to cut losses on one that is clearly off track. As a rough, non-binding reference point, a running intent well past ~40 tool calls or ~5 minutes with nothing new to show is usually worth a look -- judge from what the trace actually shows, not the numbers alone, and do nothing extra when the burn figure is absent (the setting is off, or the intent was only just claimed).
   - **Treat a new reachability edge as a route-diversity signal (when visible)**: when an admin has turned the setting on, asset_reachability above lists confirmed can_reach edges between assets (a finding showed one asset can reach another, e.g. via SSRF, a pivot, or a leaked credential's scope). Treat a newly reachable asset the same way an uncovered entry surface would be treated under "keep the routes diverse": check whether it is already covered by an open/running/recent_done intent, and if not, consider dispatching an intent against it. This does not create a new goal on its own -- it only re-prioritizes where the existing goal is pursued next. Do nothing extra when asset_reachability is absent or empty.
   - **Treat a superseded fact as a revival signal (when visible)**: when an admin has turned the setting on, fact_supersedes above lists entries {new_fact_id, old_fact_id, old_intent_id} -- a later fact that has overturned an earlier one's conclusion, and (when present) the intent that earlier fact came from. If old_intent_id is done/blocked/exhausted because of that now-overturned conclusion and the direction is still in scope and worth pursuing, dispatch a new intent with add_intent(parent_ids=[new_fact_id], ...) explaining what changed -- look it up with node_detail if it is not already in open/running/recent_done_intents. This does not reopen the old intent or create a new goal on its own -- it only flags a direction worth a fresh look. Do nothing extra when fact_supersedes is absent or empty.
   - **Treat a root-cause cluster as a dedup signal (when visible)**: when an admin has turned the setting on, root_cause_clusters above lists groups of confirmed findings {finding_ids: [...]} that a worker has explicitly linked as tracing back to the same underlying flaw. Before dispatching a new intent whose goal is to find more impact of a cause a cluster already covers, check whether the cluster already represents that direction well enough -- if so, prefer an intent that deepens or extends an existing finding's evidence over one that will likely just surface another member of the same cluster. This does not block any worker from investigating or reporting, and it does not merge or demote any finding -- it only helps avoid knowingly re-deriving an already-identified root cause. Do nothing extra when root_cause_clusters is absent or empty.
   - **Treat an unused relevant skill as a coverage signal (when visible)**: when an admin has turned the setting on, unused_relevant_skills above lists skills {skill, matched_terms} whose name/description shares vocabulary with this task's goal(s) but that no worker has successfully invoked yet. This is a coarse keyword match, not a guarantee the skill truly fits -- read matched_terms and judge for yourself. When it looks genuinely relevant and the matching goal is still open, consider using add_hint to point the next worker at it by name. This does not force any worker to use a skill, does not create a new goal on its own, and does not mean an unlisted skill is irrelevant. Do nothing extra when unused_relevant_skills is absent or empty.
   - **Check cross-task knowledge before a familiar-looking direction**: when a candidate direction resembles a generic technique or stack behaviour (default credentials, an SSRF sink, a gateway/WAF quirk, a framework's known bug class), call search_global_knowledge(q) first -- it searches confirmed facts/findings from [other tasks] and returns what another hunt already learned (a bypass variant, a dead end, an environment quirk). Use it to seed or sharpen the intent rather than re-discovering the same lesson from zero. The hits are hints from other targets: verify them against this target before relying on them, and never let a hit override evidence already gathered here.

   **Serial exploitation chains: dispatch step by step, do not split them into parallel work.** For a strongly dependent serial chain ((1)->(2)->(3), where each step depends on the previous step's actual output): do not dispatch them all in parallel (downstream work that cannot get a prerequisite that does not exist yet only duplicates or spins); record the whole chain as a todo list with TodoWrite (one item per step), dispatch only the step whose "prerequisite is satisfied" this round (usually the first), and once it produces a fact, dispatch the next on the following wake-up (the prompt will carry the todo list) and mark the satisfied ones completed. Do not split "one thing" into two ("confirm the trigger point" and "trigger the trigger point" are one step); use parallel intents only for [parallel, mutually independent] dimensions (such as enumerating several unrelated endpoints).

5. **Submit**: use [one] add_intent call to submit the new directions in a batch (the intents array, at most the 4 highest-value ones; do not call it repeatedly one at a time):
   - **A rejected intent is not a wording problem**: add_intent refuses a direction whose summary closely matches a live (open/running/paused) intent and names that intent. Do not reword the same direction to get it past the check -- either make it genuinely different (say what is uncovered that the live one does not test), adjust the live one with steer_work, or wait for it to finish. Finished intents (done/exhausted/blocked) are never blocked: re-running a direction after new evidence stays allowed.
   - **summary**: one natural-language sentence describing the direction (the full target address + what to do + why), without forcing it into a fixed category; deduplication relies mainly on comparing it against existing intents.
   - **asset_ids**: the target asset ids this direction will test/attack (pass them whenever possible, 0/1/many, from list_assets) -- whenever the direction revolves around specific assets (sites/endpoints/parameters/hosts) they must be passed, for coverage deduplication and for wiring into the asset chain; pass all of them when it spans several assets, and leave it empty only for purely global recon with no specific asset.
   - **parent_ids**: which upstream nodes this direction was derived from (optional, 0/1/many) -- pass them all when several facts combine into one intent, pass the id when it derives from an upstream intent/finding, and leave it empty for a brand new top-level direction.

Do not duplicate and do not pad; but when the goal is unachieved and a deeper uncovered play exists, dispatch it. Be concise, focused and efficient.`

func plannerSystem(goal, dataDir, workDir string) string {
	body := renderSystem("planner", plannerDefaultTmpl, PlannerVars{Goal: goal, DataDir: dataDir, Now: nowStr()})
	return body + artifactSpec(workDir)
}

// Plan runs one planning round. emit, if non-nil, receives the planner's execution
// steps (so users can see how it reads the situation and judges goals — the
// planner is the intent generator and was previously a black box). Returns whether
// the planner judged the goal met.
// triggers carries the concrete change(s) that fired this round — worker(s) done
// and/or finding(s) reported (may be several — the engine debounces a burst; empty
// for time/heartbeat wakes). They are spelled out at the top of the prompt so the
// planner looks first at the actual change (which intent, its output/finding).
func (p *Planner) Plan(ctx context.Context, taskID int64, as *db.AssetStore, ts *db.ExplorationStore, goal string, triggers []TriggerEvent, emit func(db.Activity)) (met bool, reason string, err error) {
	// cold-digest §2.3/§7: advance this task's planner-round counter, maintain the
	// cold_since_round stamps, and (if a threshold is hit) kick off background
	// compaction. Synchronous part is cheap (a few queries); the LLM compaction
	// runs in a detached goroutine so it never adds latency to this round.
	p.compactor.OnPlannerRound(ctx, ts)
	tsx := NewToolSet(ts, "planner")
	tsx.SetFindingRecorder(p.findingRecorder)
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.SetCoverageEnabled(as == nil || as.CoverageEnabled(taskID))
	tsx.killWork = p.killWork   // enable kill_work tool (nil = unavailable)
	tsx.steerWork = p.steerWork // enable steer_work tool (nil = unavailable)
	if origin, _ := ts.OriginFactID(); origin > 0 {
		tsx.SetOwnerNode(origin) // planner-side anchors default to the task root (origin fact)
	}
	// Domain tools + the basic default tool set (Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash)
	// When the asset coverage feature is off, add_task_scope/list_untested_assets are removed (and never enter the prompt).
	base := append(tsx.DropCoverageTools(tsx.PlannerTools()), actool.DefaultTools()...)
	ctx = WithRunInfo(ctx, RunInfo{TaskID: taskID, ExplorationID: explorationID(ts)})
	tools, def, cleanup := AugmentTools(ctx, "planner", base)
	defer cleanup()
	// The key state (the just-finished intent + the prefetched full graph) is moved into [this round's user
	// input] (see input below) and system keeps only the static planning body. Moving it out keeps system
	// stable every round, which helps caching; the cost is that if a single round grows long, the state may
	// be compacted (a planner round is usually short, so the risk is low). situational is appended to input below.
	situational := renderTriggers(ts, triggers) + renderGraphOverview(tsx.graphOverviewData())
	// The task-level deadline / endgame mode (injected through ctx, see taskclock.go). On the endgame round,
	// the task-timeout planner wrap-up wording is appended to this round's user input (alongside situational) as
	// [this round's operating instructions], so it only makes the final
	// goal judgement and produces no new intents.
	tc := taskClockFrom(ctx)
	if tc.Final {
		situational += "\n\n[Task endgame wrap-up (special instructions for this round, overriding the normal planning procedure above)]: " + resolveTaskTimeoutWrapup("planner")
	}
	// This task's working directory <workDir>/tasks/<taskID>, created first.
	taskDir := ensureRunDir(p.workDir, taskID, 0)
	ctx = intercept.WithReviewContext(ctx, taskDir, intercept.ReviewBackground{})
	sysBody := plannerSystem(goal, p.workDir, taskDir)
	if p.wantConstraints() {
		sysBody += constraintBlock(ts) // operating constraints (if any) injected into the system prompt to bound the exploration
	}
	system, boundary := deferredSystem(sysBody, def)
	// The planner has no wall-clock budget of its own; when there is a deadline, MaxDuration is clamped to the
	// remaining time so a planning round in flight wraps up when the task's time is up (wrap-up wording by timeout -> the task timeout wording, by step count -> the per-run wording).
	maxDur, clamped := clampMaxDuration(tc.DeadlineUnix, 0)
	settle := wrapupSettlement("planner", nil)
	if tc.DeadlineUnix > 0 {
		settle = wrapupSettlementForTask("planner", nil, clamped)
	}
	opts := agentcore.Options{
		Provider:        p.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, // goes through the recording proxy so there is a trail; the proxy CA is loaded to verify the HTTPS certificate re-signed by MITM
		WebFetchProxy:   p.proxyAddr,
		WebFetchCACert:  p.proxyCACert,
		// Web search (optional). ddgs needs no key; brave-free needs BraveKey; tavily needs TavilyKey.
		// WebSearchProxy is a separate egress proxy (http/https/socks5), unrelated to the traffic-recording MITM proxy; empty means a direct connection.
		EnableWebSearch:       p.webSearch.Enabled,
		WebSearchBackend:      p.webSearch.Backend,
		BraveSearchAPIKey:     p.webSearch.BraveKey,
		TavilySearchAPIKey:    p.webSearch.TavilyKey,
		DeepSeekSearchBaseURL: p.webSearch.DeepSeekBaseURL,
		DeepSeekSearchAPIKey:  p.webSearch.DeepSeekAPIKey,
		DeepSeekSearchModel:   p.webSearch.DeepSeekModel,
		WebSearchProxy:        p.webSearch.Proxy,
		BashEnv:               proxyEnv(p.proxyAddr, p.proxyCACert), // Bash subcommands go through the proxy and trust the CA by default
		WorkingDir:            taskDir,                              // this task's working directory <workDir>/tasks/<taskID>
		ToolOutputDir:         cmdOutDir(taskDir),
		MaxTurns:              p.maxTurns, // 0 = unlimited (configurable in agent management)
		MaxDuration:           maxDur,     // 0 = unlimited; with a deadline = the time remaining until it
		Compaction:            compactionConfig(p.compactionWindow()),
		// The planning todo list shared across wake-ups: it keeps a serial chain alive across rounds (the session is new, the store is not).
		Todos: p.todoFor(ts.ID()),
		// Hitting [this round's] step budget -> the SDK runs the wrap-up: persist the conclusions already
		// reached this round (the add_intent that should be dispatched, the prove_goal that can be proved, the
		// serial chain recorded with TodoWrite) rather than stopping planning -- the planner will keep being woken afterwards.
		// When clamped (squeezed by the task deadline), PromptByReason is used instead (see wrapupSettlementForTask).
		Settlement:   settle,
		NonStreaming: p.nonStreaming(), // use Provider.Complete when the profile selects non-streaming
		MaxTokens:    p.maxTokens(),    // 0 = do not send a cap and let the server default decide
	}
	if p.tx != nil { // persist raw LLM conversation; one accumulating file per task's planner
		opts.Transcript = p.tx
		opts.SessionID = fmt.Sprintf("exp%d-planner", ts.ID())
	}
	// Experimental: when enabled, noa takes over context compaction (archives are collected under <workDir>/noa/<SessionID> and persist).
	noaSession := fmt.Sprintf("exp%d-planner", ts.ID())
	enableNoa(&opts, p.noaEnabledFn, p.workDir, noaSession, noaWarn(noaSession))
	// The state (the just-finished intent + the full graph) is now appended to this round's user input (see
	// input below). The user message also carries the instructions + the cross-wake-up todo list (the todo list is the model's own planning scratchpad, is regenerable, and belongs in user).
	// The opening line comes in two forms depending on whether anything concrete changed this round: something
	// changed -> point at the [what actually changed] block below; nothing changed (a heartbeat sweep / a hint
	// / a resume etc.) -> do not falsely claim "the graph changed", and instead suggest reviewing running intents in passing.
	lead := "Something concrete just changed (see [what actually changed to trigger this round] below); plan the next step from it: "
	if len(triggers) == 0 {
		lead = "This round is a wake-up from a **scheduled sweep (the heartbeat fired) / with no concrete change signal** -- the graph may well have nothing new. Review the running intents in passing: use steer_work to correct one that has made no progress for a long time or gone off track, and kill_work to cut losses on one whose direction is entirely wrong; then judge the goals and decide whether to add directions: "
		// On a heartbeat / no-change wake-up, if the whole graph has no open or running intent -> exploration has
		// stalled (no worker running and no queued direction). Tell the planner explicitly and require it to add
		// a new direction this round, rather than spinning a round after only reviewing running intents.
		if active, err := ts.HasActiveIntent(); err == nil && !active {
			lead = "This round is a wake-up from a **scheduled sweep (the heartbeat fired)**, and there is currently **no open or running intent at all** -- no worker is running and no direction is queued, so exploration has stalled. You **must** produce one or more new intents this round that advance the goal and **do not duplicate** any existing intent in the graph (producing 0 intents is not allowed); first judge from the state below whether the goal is achieved, and if it is not, add directions immediately: "
		}
	}
	input := lead + situational + "\n\nFrom the state above, judge the goals. When a goal is [genuinely achieved] (the target result is in hand / the target vulnerability is confirmed), mark them one by one with prove_goal. **Hard floor: whenever the goal is not yet achieved and there is no open or running intent at all (frontier_open=0 and running_intents empty), this round must produce at least one intent that advances the goal -- at that point there is no running work to wait for and no queued direction, so producing 0 intents means the task stalls. Only when open/running intents are already making progress, or the goal is achieved, may this round produce no new intent.**" +
		renderPlannerTodos(opts.Todos.List())
	// MaxDuration now interrupts a running tool when the wall clock is up and wraps up in place (on the live
	// ctx), so a stuck round no longer bypasses the wrap-up and no external hard ctx backstop is needed. ctx only carries pause / kill / shutdown.
	_, _, err = captureRun(ctx, opts, input,
		func(r db.Activity) {
			if emit != nil {
				r.Worker = "planner" // planner activity has no intent_id (it generates them)
				emit(r)
			}
		})
	return tsx.GoalMet, tsx.Reason, err
}
