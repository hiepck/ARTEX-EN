package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
)

// jsonResult marshals v to a JSON tool result.
func jsonResult(v any) (actool.Result, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return actool.Errorf(err.Error()), nil
	}
	return actool.Text(string(b)), nil
}

// This file implements P2, the "cross-task orchestration tool set" (docs/benchmark-orchestration SS2 P2).
// These are host tools -- they need
// access to the Manager (any task's Store), the Engine (pausing) and the task creation flow, so they live in the server layer.
// The read tools redirect an "existing per-task tool" onto the target task's store (building a
// temporary ToolSet and Calling its corresponding tool), reusing exactly the same logic; the control tools (spawn/pause) call the Manager/Engine directly.
// Like the traffic tools they are seeded into the tools table and bound per agent (so only an orchestration agent sees them).

// hostTools is the runtime host-tool provider fed to ToolAugment: traffic tools
// (gated by capture) + cross-task orchestration tools + user-defined custom tools.
// The second return is the names of custom tools flagged `deferred` (schema
// withheld, routed via SearchExtraTools/ExecuteExtraTool). Per-agent binding still
// decides who actually sees any of them.
//
//nolint:unused // used as the hostTools provider in wireAgentAugment
func (s *Server) hostTools() ([]actool.CoreTool, map[string][]string) {
	tools := append(s.m.HostTools(), s.orchestrationTools()...)
	tools = append(tools, s.findingRetestTools()...)
	tools = append(tools, s.taskSubmissionReviewTools()...)
	tools = append(tools, s.platformTools()...) // platform operation tools (creating/editing skills/tools/MCP, for Auto)
	custom, err := s.customTools()
	if err != nil {
		log.Printf("[custom-tool] failed to load: %v", err)
		return tools, nil
	}
	tools = append(tools, custom...)
	// deferred custom tools → name -> its bound agent keys. ToolAugment turns a
	// name into a deferred entry only for agents it's actually bound to (so we don't
	// advertise a tool the per-agent binding will drop from the callable set).
	deferred := map[string][]string{}
	rows, _ := s.m.pg.ListCustomTools()
	for _, t := range rows {
		if t.Deferred && t.Enabled {
			deferred[t.Key] = t.Agents
		}
	}
	return tools, deferred
}

// orchestrationTools returns the cross-task tool set. Bound per-agent via the
// tools table (default: no binding — opt-in for orchestration agents).
func (s *Server) orchestrationTools() []actool.CoreTool {
	return []actool.CoreTool{
		s.toolListTasks(),
		s.toolListLLMProfiles(),
		s.toolSpawnTask(),
		s.toolPauseTask(),
		s.toolGetTaskGraph(),
		s.toolListTaskFindings(),
		s.toolAddHint(),
		s.toolGetWorkerTrace(),
		s.toolListWorkerTraces(),
		s.toolSearchWorkerTraces(),
		s.toolGetTaskNodeDetail(),
		s.toolUpdateFindingReport(),
		s.toolGetFindingTraffic(),
		s.toolBindFindingTraffic(),
	}
}

// --- schema helpers ---

func strParam(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// parseProfileID reads an LLM profile id from a tool arg that may arrive as a JSON
// number (5) or a numeric string ("5"); returns 0 when absent/unparseable.
func parseProfileID(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		v, _ := strconv.ParseInt(strings.TrimSpace(str), 10, 64)
		return v
	}
	return 0
}

func objSchema(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		req := make([]any, len(required))
		for i, r := range required {
			req[i] = r
		}
		m["required"] = req
	}
	return m
}

func roTool(name, desc string, schema map[string]any, run func(context.Context, json.RawMessage) (actool.Result, error)) actool.CoreTool {
	return actool.Build(actool.Spec{
		Name: name, Description: desc, Schema: schema,
		ReadOnly:   func(json.RawMessage) bool { return true },
		Concurrent: func(json.RawMessage) bool { return true },
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(ctx context.Context, in json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
			return run(ctx, in)
		},
	})
}

func wrTool(name, desc string, schema map[string]any, run func(context.Context, json.RawMessage) (actool.Result, error)) actool.CoreTool {
	return actool.Build(actool.Spec{
		Name: name, Description: desc, Schema: schema,
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(ctx context.Context, in json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
			return run(ctx, in)
		},
	})
}

// delegateToTask resolves the `task_id` in the input, builds a ToolSet bound to
// that task's store, strips task_id, and calls the chosen per-task tool — so the
// cross-task read reuses the exact in-task logic against another task.
func (s *Server) delegateToTask(ctx context.Context, in json.RawMessage, pick func(*agent.ToolSet) actool.CoreTool) (actool.Result, error) {
	var head struct {
		TaskID string `json:"task_id"`
	}
	_ = json.Unmarshal(in, &head)
	if strings.TrimSpace(head.TaskID) == "" {
		return actool.Errorf("task_id is required"), nil
	}
	t, ok := s.m.Task(head.TaskID)
	if !ok {
		return actool.Errorf("task does not exist: " + head.TaskID), nil
	}
	var m map[string]json.RawMessage
	_ = json.Unmarshal(in, &m)
	delete(m, "task_id")
	inner, _ := json.Marshal(m)
	tsx := agent.NewToolSet(t.Store, "orchestrator")
	if s.m.Assets() != nil {
		tsx.SetAssetStore(s.m.Assets(), s.m.Assets().Companies())
	}
	tsx.SetNotify(t.Notify)         // the generic wake-up (write operations with no dedicated callback use it; a no-op for read tools)
	tsx.SetNotifyHint(t.NotifyHint) // add_hint -> record a "a human added N strategic hints: ..." trigger and wake the planner
	return pick(tsx).Call(ctx, inner, nil)
}

// --- tools ---

func (s *Server) toolListTasks() actool.CoreTool {
	return roTool("list_tasks",
		"List every task (id/description/goal/status/run duration/parent task/LLM configuration); the orchestration agent uses it to see the whole picture, which tasks have been stuck too long, and which LLM each uses. Run duration: for a running task = creation -> now, for a terminal one = creation -> the last activity (seconds). llm_profile: the configuration name used by the task's planner/worker, where (active configuration) means it follows the global active one.",
		objSchema(map[string]any{}),
		func(context.Context, json.RawMessage) (actool.Result, error) {
			lastAct, _ := s.m.PG().LastActivityAll()
			// id -> name to resolve each task's pinned LLM profile.
			profName := map[int64]string{}
			if profs, err := s.m.pg.ListProfiles(); err == nil {
				for _, p := range profs {
					profName[p.ID] = p.Name
				}
			}
			out := make([]map[string]any, 0)
			for _, t := range s.m.List() {
				status := s.deriveTaskStatus(t)
				end := lastAct[t.ExpID]
				if live := s.engine.LastActivity(t.ID); live > end {
					end = live
				}
				dur := int64(0)
				if status == "running" {
					dur = time.Now().Unix() - t.CreatedAt
				} else if end > t.CreatedAt {
					dur = end - t.CreatedAt
				}
				row := map[string]any{"id": t.ID, "description": t.Description, "goal": t.Goal, "status": status, "run_seconds": dur}
				if t.ParentRef != "" {
					row["parent_ref"] = t.ParentRef
				}
				llmState := t.llmStateSnapshot()
				if llmState.ProfileID == nil {
					row["llm_profile"] = "(active configuration)"
				} else if n, ok := profName[*llmState.ProfileID]; ok {
					row["llm_profile"] = n
				} else {
					row["llm_profile"] = fmt.Sprintf("#%d(deleted)", *llmState.ProfileID)
				}
				out = append(out, row)
			}
			return jsonResult(out)
		})
}

// toolListLLMProfiles lists the available LLM profiles (name/model/active) so an
// orchestration agent can pick one for spawn_task's llm_profile. Never leaks keys.
func (s *Server) toolListLLMProfiles() actool.CoreTool {
	return roTool("list_llm_profiles",
		"List the available LLM configurations (profiles): id, name, model, format and whether it is the currently active one. Use the id in spawn_task's llm_profile_id parameter to give a subtask its own LLM (such as a cheap model for recon and a strong one for exploitation). It never includes API keys.",
		objSchema(map[string]any{}),
		func(context.Context, json.RawMessage) (actool.Result, error) {
			profs, err := s.m.pg.ListProfiles()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			out := make([]map[string]any, 0, len(profs))
			for _, p := range profs {
				out = append(out, map[string]any{
					"id": p.ID, "name": p.Name, "model": p.Model, "format": p.Format, "is_active": p.IsDefault,
				})
			}
			return jsonResult(map[string]any{"profiles": out})
		})
}

func (s *Server) toolSpawnTask() actool.CoreTool {
	return wrTool("spawn_task",
		"Create a subtask and start its exploration engine, returning the task_id. Use it to dispatch one thing (a challenge, a target) as an independent task. parent_ref is optional: pass the current orchestration's parent task id to link them as parent and child.",
		objSchema(map[string]any{
			"description":            strParam("the task description (a short title)"),
			"goal":                   strParam("the task goal (what is to be achieved)"),
			"parent_ref":             strParam("optional: the parent task id (to link them as parent and child)"),
			"source_task_ids":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": fmt.Sprintf("optional: the list of source task ids to inherit read-only (at most %d). The subtask can reference those tasks' established assets/conclusions read-only as a starting point; unlike parent_ref, which is a pure parent/child pointer, this is content inheritance.", db.MaxTaskSourceCount)},
			"llm_profile_id":         map[string]any{"type": "integer", "description": "optional: the id of the LLM configuration this subtask's planner/worker uses (see list_llm_profiles); leave it empty to inherit from the parent task and then fall back to the globally active configuration"},
			"timeout_seconds":        map[string]any{"type": "integer", "description": "optional: the task-level timeout (seconds). When it is reached, a graceful wrap-up is triggered and the task enters the timeout terminal state; empty or 0 = unlimited"},
			"plan_heartbeat_seconds": map[string]any{"type": "integer", "description": "optional: the planner heartbeat interval (seconds). Once this long has passed since the last planning round ended / the task started with no trigger in between -> one planning round is triggered (a deadlock backstop + a wake-up to supervise in-flight workers). Empty or 0 = the default 600 (10min);"},
			"seed_first_intent":      map[string]any{"type": "boolean", "description": "optional: can be enabled for a simple task so that a seed intent (its content = the description + goal) is dispatched on creation and a worker can start testing immediately without waiting for the first planner round; false by default (the standard plan-then-execute flow)."},
		}, "description", "goal"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Description          string          `json:"description"`
				Goal                 string          `json:"goal"`
				ParentRef            string          `json:"parent_ref"`
				SourceTaskIDs        []string        `json:"source_task_ids"`
				LLMProfileID         json.RawMessage `json:"llm_profile_id"`
				TimeoutSeconds       int             `json:"timeout_seconds"`
				PlanHeartbeatSeconds int             `json:"plan_heartbeat_seconds"`
				SeedFirstIntent      bool            `json:"seed_first_intent"`
			}
			_ = json.Unmarshal(in, &a)
			if strings.TrimSpace(a.Description) == "" {
				a.Description = "untitled task"
			}
			if strings.TrimSpace(a.Goal) == "" {
				return actool.Errorf("goal is required"), nil
			}
			if a.TimeoutSeconds < 0 {
				a.TimeoutSeconds = 0
			}
			// Read-only inherited source tasks: the count cap + each id valid/deduplicated/existing, validated by the same rules as HTTP task creation.
			if len(a.SourceTaskIDs) > db.MaxTaskSourceCount {
				return actool.Errorf(fmt.Sprintf("at most %d linked tasks may be selected", db.MaxTaskSourceCount)), nil
			}
			sourceIDs := make([]int64, 0, len(a.SourceTaskIDs))
			seenSources := map[int64]bool{}
			for _, raw := range a.SourceTaskIDs {
				id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
				if err != nil || id <= 0 || seenSources[id] {
					return actool.Errorf("a linked task id is invalid or duplicated"), nil
				}
				if _, ok := s.m.Task(strconv.FormatInt(id, 10)); !ok {
					return actool.Errorf(fmt.Sprintf("linked task #%d does not exist", id)), nil
				}
				seenSources[id] = true
				sourceIDs = append(sourceIDs, id)
			}
			// LLM profile resolution: explicit id > inherit parent's pin > active(nil).
			var pin *int64
			if id := parseProfileID(a.LLMProfileID); id > 0 {
				if _, ok := s.loadProfileConfig(id); !ok {
					return actool.Errorf(fmt.Sprintf("LLM configuration #%d does not exist or has no API key set", id)), nil
				}
				pin = &id
			} else if a.ParentRef != "" {
				if pt, ok := s.m.Task(a.ParentRef); ok {
					pin = pt.LLMProfileID
				}
			}
			var llmIDs []int64
			if pin != nil {
				llmIDs = []int64{*pin}
			}
			t, err := s.m.CreateTaskWithOptions(a.Description, a.Goal, db.TaskCreateOptions{
				SourceTaskIDs:        sourceIDs,
				LLMProfileIDs:        llmIDs,
				TimeoutSeconds:       a.TimeoutSeconds,
				PlanHeartbeatSeconds: a.PlanHeartbeatSeconds,
			})
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if a.ParentRef != "" {
				t.ParentRef = a.ParentRef
				if id, e := strconv.ParseInt(t.ID, 10, 64); e == nil {
					_ = s.m.PG().SetParentRef(id, a.ParentRef)
				}
			}
			// The shared post-creation flow, reusing the same launchTask as HTTP task creation (server.go createTask):
			// seed + visible background goal decomposition (round 0 / the LLM step / each goal) + engine.Run.
			// seed_first_intent is false by default (the standard plan-then-execute flow); a simple task can enable it to dispatch one work straight away for testing.
			s.launchTask(t, a.Description+" "+a.Goal, a.SeedFirstIntent)
			return actool.Text(fmt.Sprintf("task created: %s", t.ID)), nil
		})
}

func (s *Server) toolPauseTask() actool.CoreTool {
	return wrTool("pause_task", "Pause the named task (stopping its planner/worker loops).",
		objSchema(map[string]any{"task_id": strParam("the id of the task to pause")}, "task_id"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				TaskID string `json:"task_id"`
			}
			_ = json.Unmarshal(in, &a)
			t, ok := s.m.Task(a.TaskID)
			if !ok {
				return actool.Errorf("task does not exist: " + a.TaskID), nil
			}
			if _, err := s.applyTaskControlWithCause(t, "pause", agent.AbortPausedByOrchestrator); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text("task paused: " + a.TaskID), nil
		})
}

func (s *Server) toolGetTaskGraph() actool.CoreTool {
	return roTool("get_task_graph", "Read the named task's exploration graph overview (the same as graph_overview: asset counts / frontier / findings / coverage and so on); name the task with task_id.",
		objSchema(map[string]any{"task_id": strParam("the task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).GraphOverviewTool)
		})
}

func (s *Server) toolListTaskFindings() actool.CoreTool {
	return roTool("list_task_findings", "Read the named task's confirmed findings (including flags/PoCs; each entry carries id/task_id/intent_id/vulnclass/severity/summary/status); name the task with task_id.",
		objSchema(map[string]any{"task_id": strParam("the task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).ListFindingsTool)
		})
}

func (s *Server) toolAddHint() actool.CoreTool {
	return wrTool("add_task_hint", "Inject a strategic hint into the named task (that task's planner reads it when generating intents next round).\n"+
		"* Prefer batching: put several hints into the hints array and submit them at once (it returns an ids array of the same length and order as hints, with id=0 for a failed entry); for a single one, omit hints and give the top-level text directly.",
		objSchema(map[string]any{
			"task_id":      strParam("the task id"),
			"hints":        map[string]any{"type": "array", "description": "[prefer this] the array of hints; each element has the same fields as the top level (text/asset_ids/traffic_refs).", "items": objSchema(map[string]any{"text": strParam("the hint content"), "asset_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}, "traffic_refs": agent.HintTrafficSchema()})},
			"text":         strParam("[single] the hint content"),
			"traffic_refs": agent.HintTrafficSchema(),
			"asset_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "the anchored asset ids (optional, 0/1/many; asset ids within that task)"},
		}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).AddHintTool)
		})
}

func (s *Server) toolGetWorkerTrace() actool.CoreTool {
	return roTool("get_task_worker_trace",
		"Look at the execution trace of one work (intent) in the named task: get_task_worker_trace(task_id, intent_id) gives the step summaries; adding step_ids=[...] fetches the full content of those steps (at most 5 per call, beyond which only the first 5 are returned).",
		objSchema(map[string]any{
			"task_id":   strParam("the task id"),
			"intent_id": map[string]any{"type": "integer", "description": "the intent id (a work within that task)"},
			"step_ids":  map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "optional: the step ids whose full content to fetch (at most 5 per call, beyond which only the first 5 are returned and the rest are listed in omitted_step_ids)"},
		}, "task_id", "intent_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).GetWorkerTraceTool)
		})
}

func (s *Server) toolListWorkerTraces() actool.CoreTool {
	return roTool("list_task_worker_traces", "List which work (intents) have run in the named task plus their step counts, to discover which work is worth looking at (then use get_task_worker_trace).",
		objSchema(map[string]any{"task_id": strParam("the task id")}, "task_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).ListWorkerTracesTool)
		})
}

func (s *Server) toolSearchWorkerTraces() actool.CoreTool {
	return roTool("search_task_worker_traces", "Search every work's execution trace in the named task by keyword (returning the matching step summaries + intent_id).",
		objSchema(map[string]any{"task_id": strParam("the task id"), "q": strParam("the search keyword")}, "task_id", "q"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).SearchWorkerTracesTool)
		})
}

func (s *Server) toolGetTaskNodeDetail() actool.CoreTool {
	return roTool("get_task_node_detail",
		"Read the full content of one exploration graph node in the named task (a finding/fact/intent/goal: the summary + details/evidence/PoC). id is the exploration node id (as returned by report_finding, or the id in list_task_findings). Use it to fetch a finding's complete evidence before writing its report.",
		objSchema(map[string]any{
			"task_id": strParam("the task id"),
			"id":      map[string]any{"type": "integer", "description": "the exploration graph node id (not an asset id)"},
		}, "task_id", "id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			return s.delegateToTask(ctx, in, (*agent.ToolSet).NodeDetailTool)
		})
}

// toolUpdateFindingReport writes/overwrites a finding's detailed Markdown report.
// finding_id is the id report_finding returned ("finding recorded: <id>", the
// finding node id). The write (SetFindingReportByNodeID) is keyed by node_id and
// task-agnostic, so this host tool needs no task_id / exploration store.
func (s *Server) toolUpdateFindingReport() actool.CoreTool {
	return wrTool("update_finding_report",
		"Write or update the **detailed report** of a registered finding (the full Markdown, replacing the old content entirely). finding_id takes the id report_finding returned (the number in \"finding recorded: <id>\"). The report should cover: an overview of the vulnerability, its impact, reproduction steps, evidence/PoC and remediation advice.",
		objSchema(map[string]any{
			"finding_id":       map[string]any{"type": "integer", "description": "the target finding's id (the id report_finding returned)"},
			"report":           strParam("the full detailed report, in Markdown"),
			"evidence_version": map[string]any{"type": "integer", "description": "the evidence version returned by get_finding_traffic; it stops a report overwriting a newer evidence change"},
		}, "finding_id", "report"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				EvidenceVersion *int64          `json:"evidence_version"`
				FindingID       json.RawMessage `json:"finding_id"`
				Report          string          `json:"report"`
			}
			_ = json.Unmarshal(in, &a)
			nodeID := parseProfileID(a.FindingID) // reuse the "a number or a numeric string" parser
			if nodeID <= 0 {
				return actool.Errorf("finding_id is invalid"), nil
			}
			n, err := s.m.pg.SetFindingReportVersionByNodeID(ctx, nodeID, a.Report, a.EvidenceVersion)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if n == 0 {
				return actool.Errorf(fmt.Sprintf("no finding record was found for finding_id=%d (register it with report_finding first)", nodeID)), nil
			}
			return actool.Text(fmt.Sprintf("finding %d report updated (%d chars)", nodeID, len(a.Report))), nil
		})
}

// deriveTaskStatus mirrors listTasks' status derivation for the list_tasks tool.
func (s *Server) deriveTaskStatus(t *Task) string {
	lifecycle := t.lifecycleSnapshot()
	switch {
	case isTerminalStatus(lifecycle.Status):
		return lifecycle.Status
	case lifecycle.Paused || s.engine.IsPaused(t.ID):
		return "paused"
	case s.engine.ReadyFor(t) && s.engine.Started(t.ID):
		return "running"
	}
	return "created"
}

// orchestrationToolSeeds seeds the cross-task tools into the tools table so they
// are bindable per-agent (default: bound to nobody — opt-in for orchestration
// agents). First-insert only, like the traffic seeds.
func (s *Server) seedOrchestrationTools() {
	// task-op + platform tools default-bind to the built-in Auto agent (which exists
	// to operate the platform). SeedTool only takes effect on first insert; rows an old database already seeded are bound by seedAutoDefaultBindings.
	autoAgents, _ := json.Marshal([]string{"auto"})
	for _, t := range s.orchestrationTools() {
		schema, _ := json.Marshal(t.InputSchema())
		bindings := autoAgents
		if t.Name() == "bind_finding_traffic" {
			bindings = json.RawMessage(`["reporter"]`)
		}
		_ = s.m.PG().SeedTool(t.Name(), t.Description(), schema, bindings)
	}
	for _, t := range s.platformTools() {
		schema, _ := json.Marshal(t.InputSchema())
		_ = s.m.PG().SeedTool(t.Name(), t.Description(), schema, autoAgents)
	}
	s.refreshBuiltinToolSchemas()
	s.seedAutoDefaultBindings()
	s.seedPlannerDefaultBindings()
	s.seedPlannerListAssetsBinding()
	s.seedCompanyScopeRebind()
	s.seedWorkerReadToolsUnbind() // unbind list_facts/list_companies/list_worker_traces from worker by default (one-off)
	s.seedWorkerReadbackRebind()  // repair a bad old migration: rebind search_all_worker_traces/get_worker_trace/node_detail to worker (one-off)
	s.seedAutoReportFindingBinding()
	s.unbindGoalMetDefault()
	s.reseedGoalsPrompt()             // the goals prompt gained the "extract operating constraints" step -> append a new default version for old databases (one-off)
	s.reseedMainAgentPrompt()         // the mainagent prompt gained "after the goals are achieved, add_intent asks whether to register a goal" (one-off)
	s.reseedPlannerPrompt()           // the planner prompt: rewrote the legitimate reasons for "0 intents" + added the quantitative acceptance check (one-off)
	s.reseedWorkerPrompt()            // the worker prompt: added the evidence bar for a negative conclusion (one-off)
	s.seedReporterAgent()             // seed the "report writing" agent + its tool bindings + the finding trigger (one-off)
	s.upgradeReporterTriggerMessage() // a migration for old databases: make the reporter return evidence_version (one-off)
	s.seedFindingTrafficTools()       // add the optional evidence parameters and the read-only evidence tools, preserving the user's configuration
	s.seedFindingWorkflowTools()
	// Note: pentest's default tool bindings need no migration -- on a fresh initialization
	// BuiltinToolSeeds already seeds list_assets/insert_assets/report_finding/list_findings/list_companies
	// together with pentest (the project has no legacy databases, so no migration is done).
}

// refreshBuiltinToolSchemas propagates code schema/description changes on the
// orchestration + platform tools into already-seeded rows ONCE per version flag —
// SeedTool is first-insert-only, so a new param (e.g. spawn_task's llm_profile) never
// reaches an old DB otherwise. Preserves each tool's agent binding + enabled flag.
// Bump the flag whenever these tools' schemas/descriptions change in code.
func (s *Server) refreshBuiltinToolSchemas() {
	const flag = "tool_schema_refresh_v7_list_facts_paging"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	tools := append(s.orchestrationTools(), s.platformTools()...)
	for _, t := range tools {
		schema, _ := json.Marshal(t.InputSchema())
		if err := s.m.pg.RefreshToolDefaults(t.Name(), t.Description(), schema); err != nil {
			log.Printf("[tools] refresh %s schema failed: %v", t.Name(), err)
		}
	}
	// It also refreshes some builtin agent tools to the code defaults:
	//   - goal_met: the description seeded into an old database misleadingly said "end this planning
	//     round", which made the planner treat it as a way to "end an empty round" and declare the whole task complete right after starting.
	//   - insert_assets: a new related parameter (marking whether an asset is relevant to the current task,
	//     which decides whether it enters coverage);
	//     SeedTool is first-insert-only, so a schema already seeded in an old database would never receive the new parameter.
	//   - list_facts: it became paginated with new limit/before/q parameters; an empty schema already
	//     seeded in an old database would otherwise show "no parameters" on the tool management page and keep those parameter descriptions from the model.
	refreshBuiltin := map[string]bool{"goal_met": true, "insert_assets": true, "list_facts": true}
	for _, sd := range agent.BuiltinToolSeeds() {
		if !refreshBuiltin[sd.Key] {
			continue
		}
		schema, _ := json.Marshal(sd.Schema)
		if err := s.m.pg.RefreshToolDefaults(sd.Key, sd.Desc, schema); err != nil {
			log.Printf("[tools] refresh %s desc failed: %v", sd.Key, err)
		}
	}
	_ = s.m.pg.SetSetting(flag, "true")
	log.Printf("[tools] refreshed the orchestration/platform tool schemas to the code defaults (one-off)")
}

// unbindGoalMetDefault removes goal_met's default "planner" binding ONCE (guarded by
// a settings flag), so existing DBs match the new default of NO agent. goal_met bypasses
// per-goal prove_goal to declare the whole task done — powerful/risky and redundant with
// the prove_goal→auto-complete path — so it ships unbound; users can re-bind it per agent
// in the UI. A user's own binding to another agent is untouched (we only strip planner).
func (s *Server) unbindGoalMetDefault() {
	const flag = "goal_met_unbind_default_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.RemoveAgentFromTool("planner", "goal_met"); err != nil {
		log.Printf("[tools] failed to unbind goal_met from planner: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// reseedGoalsPrompt refreshes the goals decomposer's prompt to the **current code default** -- because
// the default body gained the
// "extract the operating constraints (set_constraints) before splitting the goals" step, and
// SeedPromptIfEmpty is first-insert-only, so version 1 already in an old database never receives it.
// This uses version management to **append a new version** and switch to it (ResetPromptToDefault),
// keeping the old version in the history so a user who customized it can recover it. A settings flag guards it -> it runs once;
// bump that flag when the default changes again. A fresh database needs nothing (SeedPromptIfEmpty already seeded the latest default).
func (s *Server) reseedGoalsPrompt() {
	const flag = "goals_prompt_constraint_step_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // attempt it only once, successful or not
	a, err := s.m.pg.GetAgentByKey("goals")
	if err != nil || a == nil {
		return // on a fresh database where the agent row does not exist yet, seedPrompts seeds the latest default directly and this migration is unnecessary
	}
	tmpl := agent.BuiltinPromptSeeds()["goals"]
	if tmpl == "" {
		return
	}
	// On a fresh database seedPrompts already seeded the latest default -> the current version equals the code default, so there is no need to append a duplicate.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] failed to refresh the goals prompt to the new default: %v", err)
		return
	}
	log.Printf("[prompts] appended a new default version of the goals prompt (adding the constraint extraction step, one-off)")
}

// reseedMainAgentPrompt refreshes the mainagent prompt to the **current code default** -- the default
// body gained the guidance "when every goal is achieved and add_intent dispatches an intent directly,
// ask the human whether to register it as a formal goal", and SeedPromptIfEmpty is first-insert-only so
// a version already in an old database never receives it. It uses version management to **append a new
// version** and switch to it (ResetPromptToDefault), with the old version kept in the history so a user
// who customized it can recover it. A settings flag guards it -> it runs once. A fresh database needs nothing
// (SeedPromptIfEmpty already seeded the latest default). Structurally identical to reseedGoalsPrompt.
func (s *Server) reseedMainAgentPrompt() {
	const flag = "mainagent_prompt_goalless_intent_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // attempt it only once, successful or not
	a, err := s.m.pg.GetAgentByKey("mainagent")
	if err != nil || a == nil {
		return // on a fresh database where the agent row does not exist yet, seedPrompts seeds the latest default directly and this migration is unnecessary
	}
	tmpl := agent.BuiltinPromptSeeds()["mainagent"]
	if tmpl == "" {
		return
	}
	// On a fresh database seedPrompts already seeded the latest default -> the current version equals the code default, so there is no need to append a duplicate.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] failed to refresh the mainagent prompt to the new default: %v", err)
		return
	}
	log.Printf("[prompts] appended a new default version of the mainagent prompt (adding the ask-to-register-a-goal step after the goals are achieved, one-off)")
}

// reseedPlannerPrompt refreshes the planner prompt to the **current code default** -- the default body
// was condensed and restructured, "restraint" was downgraded to deduplication only, and
// "depth beats coverage", the "hard floor: the goal is unachieved and no intent is running, so one must
// be produced" rule and an upper bound on re-checking a negative conclusion were added.
// Bump the flag below (currently v2) whenever the default changes materially, so existing old databases
// refresh again. SeedPromptIfEmpty is first-insert-only and a version already in an old database never
// receives it, so version management is used to
// **append a new version** and switch to it (ResetPromptToDefault), with the old version kept in the history so a user who customized it can
// recover it. A settings flag guards it -> it runs once. A fresh database needs nothing (SeedPromptIfEmpty already seeded the latest default). Structurally identical to
// reseedGoalsPrompt.
func (s *Server) reseedPlannerPrompt() {
	const flag = "planner_prompt_compact_realistic_v2"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // attempt it only once, successful or not
	a, err := s.m.pg.GetAgentByKey("planner")
	if err != nil || a == nil {
		return // on a fresh database where the agent row does not exist yet, seedPrompts seeds the latest default directly and this migration is unnecessary
	}
	tmpl := agent.BuiltinPromptSeeds()["planner"]
	if tmpl == "" {
		return
	}
	// On a fresh database seedPrompts already seeded the latest default -> the current version equals the code default, so there is no need to append a duplicate.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] failed to refresh the planner prompt to the new default: %v", err)
		return
	}
	log.Printf("[prompts] appended a new default version of the planner prompt (condensed restructure + restraint downgraded to deduplication + depth first + an upper bound on negative re-checks, one-off)")
}

// reseedWorkerPrompt refreshes the worker prompt to the **current code default** -- the record_fact
// section of the default body dropped the whole "a negative conclusion states the observation plus a
// tentative reading" sentence and decoupled confidence (observed/inferred) from "whether this intent's
// techniques are exhausted" (both of which mislead the planner),
// and it tightened splitting into a facts array down to the rare exception of "completely independent,
// unmergeable" conclusions. Bump the flag to v3 so existing old databases refresh again.
// SeedPromptIfEmpty is first-insert-only and a version already in an old database never receives it, so version management is used to **append a new version** and switch to it, with the old version kept in the history and recoverable.
// A settings flag guards it -> it runs once. A fresh database needs nothing. Structurally identical to reseedGoalsPrompt.
func (s *Server) reseedWorkerPrompt() {
	const flag = "worker_prompt_compact_v4"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // attempt it only once, successful or not
	a, err := s.m.pg.GetAgentByKey("worker")
	if err != nil || a == nil {
		return // on a fresh database where the agent row does not exist yet, seedPrompts seeds the latest default directly and this migration is unnecessary
	}
	tmpl := agent.BuiltinPromptSeeds()["worker"]
	if tmpl == "" {
		return
	}
	// On a fresh database seedPrompts already seeded the latest default -> the current version equals the code default, so there is no need to append a duplicate.
	if cur, err := s.m.pg.CurrentPrompt(a.ID); err == nil && cur == tmpl {
		return
	}
	if _, err := s.m.pg.ResetPromptToDefault(a.ID, tmpl); err != nil {
		log.Printf("[prompts] failed to refresh the worker prompt to the new default: %v", err)
		return
	}
	log.Printf("[prompts] appended a new default version of the worker prompt (the context-lookup section narrowed to list_assets/list_findings, dropping list_facts/node_detail/asset_neighbors, one-off)")
}

// reporterToolCallMessage must unconditionally require reading get_finding_traffic once before writing
// the report.
// That tool is read-only and "does not depend on the capture switch", so it can read manually bound
// evidence whether automatic binding is on or off. If this said
// "read it only when automatic binding is enabled", the reporter would not pass evidence_version under
// the default-off configuration,
// SetFindingReportVersionByNodeID would then write -1 per the legacy semantics, and the finding detail
// page and the Markdown export would permanently show
// "the evidence has changed, the report is pending an update" with no UI affordance to clear it.
const reporterToolCallMessage = "A finding has just been registered by report_finding above. Read the finding_id (the standalone finding record ID) and finding_node_id (the exploration node ID) from the returned JSON, " +
	"then use get_finding_traffic(finding_id) to read the current evidence list and its version (an empty list is normal, write the report as usual); " +
	"if the operating guidance enables automatic binding, verify and link this finding's traffic before reading it. Use finding_node_id for the node details. " +
	"Finally call update_finding_report(finding_id=finding_node_id, report, evidence_version=the version actually read) to save it; " +
	"evidence_version must be passed, or the report is permanently marked as pending an update. Do not mix up the two kinds of ID."

// The legacy trigger message (0.3.8 and earlier). Only a record still identical to it word for word is overwritten by the migration; anything the user edited is left alone.
const reporterToolCallMessageV1 = "A finding has just been registered by report_finding above. Take the finding_id" +
	"(the number in the tool's \"finding recorded: <id>\" return) and the task id from the trigger context, write that finding's detailed report per your responsibilities, " +
	"and finally call update_finding_report(finding_id, report) to save it."

// upgradeReporterTriggerMessage refreshes a reporter trigger message in an old database that is still the default wording.
// seedReporterAgent is guarded by reporter_agent_seed_v1 and only writes the trigger when creating the agent, so
// an upgraded database never receives the new wording -- the tool schema was completed with
// evidence_version by seedFindingTrafficTools, but nothing told the reporter to use it. One-off, and it only overwrites wording that was never edited.
func (s *Server) upgradeReporterTriggerMessage() {
	const flag = "reporter_trigger_evidence_version_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // attempt it only once
	triggers, err := s.m.pg.ListTriggersFor("reporter")
	if err != nil {
		log.Printf("[reporter] failed to read the triggers: %v", err)
		return
	}
	for _, t := range triggers {
		if !t.OnToolCall || t.ToolCallMessage != reporterToolCallMessageV1 {
			continue // the user edited it, or it is not a finding trigger; leave it alone.
		}
		t.ToolCallMessage = reporterToolCallMessage
		if err := s.m.pg.UpdateTrigger(t); err != nil {
			log.Printf("[reporter] failed to upgrade the trigger message: %v", err)
			return
		}
		log.Printf("[reporter] the trigger message now reads and returns evidence_version")
	}
}

// seedReporterAgent seeds a "report writing" custom agent (builtin=false, so it is editable/deletable in the UI):
// it binds update_finding_report + the task query tools and attaches a trigger that fires "whenever
// report_finding is called" --
// so every registered finding wakes it to write a detailed report. One-off (guarded by a settings flag): it is not recreated once the user deletes it.
// Dependency: the orchestration tools are stored by SeedTool above in this function, so they can be bound.
func (s *Server) seedReporterAgent() {
	const flag = "reporter_agent_seed_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	defer func() { _ = s.m.pg.SetSetting(flag, "true") }() // attempt it only once, successful or not

	if exist, _ := s.m.pg.GetAgentByKey("reporter"); exist != nil {
		return // the key is already taken (the user created one by hand) -- do not overwrite it
	}
	a, err := s.m.pg.CreateAgent("reporter", "Report writing",
		"Detailed finding report writing: triggered automatically when a finding is made; it fetches the evidence and execution trace, writes a Markdown report and saves it back.")
	if err != nil {
		log.Printf("[reporter] failed to create the agent: %v", err)
		return
	}
	if err := s.m.pg.SeedPromptIfEmpty(a.ID, agent.ReporterDefaultPrompt); err != nil {
		log.Printf("[reporter] failed to seed the prompt: %v", err)
	}
	// The trigger run policy: parallel + none -- one report per finding, with several findings written concurrently.
	// merge must be none: otherwise (the default all) a wave of findings would be merged into one run and the parallelism would be pointless.
	// maxParallel=5: at most 5 report sessions at once, to avoid a burst of LLM calls.
	if err := s.m.pg.SetAgentTriggerBehavior("reporter", "parallel", "none", 5); err != nil {
		log.Printf("[reporter] failed to set the trigger run policy: %v", err)
	}
	// Bind the tools it needs: writing the report + reading the evidence/execution trace/state.
	if err := s.m.pg.AddAgentToToolBinding("reporter", []string{
		"update_finding_report", "get_task_node_detail", "list_task_findings",
		"get_task_worker_trace", "list_task_worker_traces", "search_task_worker_traces",
		"get_task_graph",
	}); err != nil {
		log.Printf("[reporter] failed to bind the tools: %v", err)
	}
	// The trigger: it fires whenever report_finding is called (the tool's "finding recorded: <id>" return carries the finding_id,
	// and the task id is in the trigger message too).
	if _, err := s.m.pg.CreateTrigger(&db.AgentTrigger{
		AgentKey:        "reporter",
		Enabled:         true,
		OnToolCall:      true,
		ToolNames:       []string{"report_finding"},
		ToolCallMessage: reporterToolCallMessage,
	}); err != nil {
		log.Printf("[reporter] failed to create the trigger: %v", err)
	}
	log.Printf("[reporter] seeded the \"report writing\" agent + the finding trigger")
}

// seedAutoReportFindingBinding adds "auto" to report_finding's binding ONCE so
// conversation-context agents can call it without requiring an intent_id.
func (s *Server) seedAutoReportFindingBinding() {
	const flag = "auto_report_finding_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("auto", []string{"report_finding"}); err != nil {
		log.Printf("[auto] failed to apply the default report_finding binding: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedPlannerDefaultBindings adds "planner" to report_finding's binding ONCE
// (guarded by a settings flag), so existing DBs — whose report_finding row was
// seeded as worker-only — also let the planner record findings. Fresh DBs already
// get it via PlannerTools(); this only backfills without overriding a user unbind.
func (s *Server) seedPlannerDefaultBindings() {
	const flag = "planner_report_finding_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"report_finding"}); err != nil {
		log.Printf("[planner] failed to apply the default report_finding binding: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedPlannerListAssetsBinding adds "planner" to list_assets's binding ONCE
// (guarded by a settings flag), so existing DBs — whose list_assets row was seeded
// as auto/pentest-only — also let the planner query the asset store by DSL. Fresh
// DBs already get it via PlannerTools(); this only backfills without overriding a
// user unbind.
func (s *Server) seedPlannerListAssetsBinding() {
	const flag = "planner_list_assets_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"list_assets"}); err != nil {
		log.Printf("[planner] failed to apply the default list_assets binding: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedCompanyScopeRebind changes add_company_scope's default binding ONCE on
// existing DBs (guarded by a settings flag): the tool moves off worker and onto
// planner — defining a company's asset scope is a planning/main/auto concern, not
// something a worker does mid-exploration. Fresh DBs already get planner via
// PlannerTools() and lack worker via WorkerTools(); this only backfills old rows.
// One-shot + flag-guarded so a user who later re-binds worker isn't overridden.
func (s *Server) seedCompanyScopeRebind() {
	const flag = "company_scope_rebind_v1" // the default binding moves from worker to planner
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("planner", []string{"add_company_scope"}); err != nil {
		log.Printf("[planner] failed to apply the default add_company_scope binding: %v", err)
		return
	}
	if err := s.m.pg.RemoveAgentFromTool("worker", "add_company_scope"); err != nil {
		log.Printf("[worker] failed to unbind add_company_scope: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedWorkerReadToolsUnbind strips the read-context tools off worker's default
// binding ONCE on existing DBs (guarded by a settings flag): a worker executes one
// intent and writes back — reading facts/companies and listing all workers' traces is
// a planning/main concern, not the executor's. Fresh DBs already lack these via
// WorkerTools(); this only backfills old rows without overriding a user who
// deliberately re-binds worker. Each RemoveAgentFromTool is per-tool +
// membership-guarded, so planner/mainagent bindings of the same tool are untouched.
//
// NOTE: search_all_worker_traces / get_worker_trace / node_detail are intentionally NOT
// unbound — worker owns them for cross-work look-back + node drill-down (see WorkerTools).
// They used to be in this list back when worker lacked them; seedWorkerReadbackRebind
// repairs DBs whose old run stripped them.
func (s *Server) seedWorkerReadToolsUnbind() {
	const flag = "worker_readtools_unbind_v1"
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	for _, k := range []string{
		"list_facts", "list_companies", "list_worker_traces",
	} {
		if err := s.m.pg.RemoveAgentFromTool("worker", k); err != nil {
			log.Printf("[worker] failed to unbind %s from worker: %v", k, err)
			return // on an error do not set the flag, so the next startup retries
		}
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedWorkerReadbackRebind re-binds the cross-work look-back / drill-down tools onto
// worker ONCE (guarded by a settings flag): an earlier seedWorkerReadToolsUnbind wrongly
// stripped search_all_worker_traces / get_worker_trace / node_detail from worker after
// they had been added to WorkerTools(), so any DB that ran that migration lost them.
// Fresh DBs already have them via WorkerTools() and this is a harmless no-op there.
// One-shot + flag-guarded so a user who later deliberately unbinds them isn't overridden.
func (s *Server) seedWorkerReadbackRebind() {
	const flag = "worker_readback_rebind_v2" // v2: node_detail added
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	if err := s.m.pg.AddAgentToToolBinding("worker", []string{
		"search_all_worker_traces", "get_worker_trace", "node_detail",
	}); err != nil {
		log.Printf("[worker] failed to rebind the look-back/detail tools: %v", err)
		return // on an error do not set the flag, so the next startup retries
	}
	_ = s.m.pg.SetSetting(flag, "true")
}

// seedAutoDefaultBindings adds "auto" to the task-op + platform tools' bindings
// ONCE (guarded by a settings flag), so existing DBs whose tool rows were seeded
// before Auto existed still give Auto its default toolset — without re-adding it
// after a user deliberately unbinds.
func (s *Server) seedAutoDefaultBindings() {
	const flag = "auto_default_bindings_v3" // v3: the old asset tool names replaced, insert_assets/add_company_scope added
	if v, _, _ := s.m.pg.GetSetting(flag); v == "true" {
		return
	}
	keys := make([]string, 0, len(platformToolKeys)+12)
	for _, t := range s.orchestrationTools() {
		keys = append(keys, t.Name())
	}
	keys = append(keys, platformToolKeys...)
	// Asset tools: Auto often needs to view/register assets and manage company scopes while operating the platform.
	keys = append(keys, "insert_assets", "add_company_scope", "list_assets")
	if err := s.m.pg.AddAgentToToolBinding("auto", keys); err != nil {
		log.Printf("[auto] failed to apply the default bindings: %v", err)
		return
	}
	_ = s.m.pg.SetSetting(flag, "true")
}
