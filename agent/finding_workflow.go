package agent

import (
	"encoding/json"
	"fmt"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

const findingIDGuidance = "\n\n**Finding ID convention**: finding_id is the standalone finding record ID; finding_node_id is the exploration node ID. The id returned by list_findings / list_task_findings / node_detail / get_task_node_detail stays the exploration node ID, and the standalone number must be read from the finding_id in the same response. get_finding_traffic / bind_finding_traffic use the standalone finding_id. The legacy update_finding_report still takes finding_node_id in its finding_id parameter. Do not use the number on report_finding's first line with the evidence tools, and do not guess another number after an ID error."

// The server supplies the persisted setting. A missing setting/host is off.
// Consulted at assembly and again on writes so an already-running session
// cannot keep binding after the user switches the feature off.
var FindingTrafficBindingEnabled func() bool

func findingTrafficBindingEnabled() bool {
	return FindingTrafficBindingEnabled != nil && FindingTrafficBindingEnabled()
}

// Applied after ToolResolve: user descriptions and prompts remain intact, while
// all actual reporters (including Planner and custom chat agents) see the same
// API contract. Disabled/unbound tools are never reintroduced here.
func findingWorkflowTools(agentKey string, tools []actool.CoreTool) ([]actool.CoreTool, string) {
	if !findingTrafficBindingEnabled() {
		out := make([]actool.CoreTool, 0, len(tools))
		for _, tool := range tools {
			if tool.Name() == "bind_finding_traffic" {
				continue
			}
			if agentKey == "reporter" && (tool.Name() == "traffic_search" || tool.Name() == "traffic_get" || tool.Name() == "traffic_blob") {
				continue
			}
			switch tool.Name() {
			case "report_finding", "add_hint", "add_task_hint":
				// Work on a copy: toggling back on must restore the original schema.
				raw, _ := json.Marshal(tool.InputSchema())
				var schema map[string]any
				if json.Unmarshal(raw, &schema) == nil {
					stripTrafficParameters(schema)
					tool = DecorateTool(tool, tool.Description(), schema)
				}
			}
			out = append(out, tool)
		}
		return stripReachesIfDisabled(out), ""
	}
	out := append([]actool.CoreTool(nil), tools...)
	has := map[string]bool{}
	for i, tool := range out {
		has[tool.Name()] = true
		note := ""
		switch tool.Name() {
		case "report_finding":
			note = "\nBy default the report agent verifies and binds the traffic before writing the report. The reporter keeps the verification commands, the key output and the IDs of existing real traffic with their roles in evidence, so the report agent can check them against the execution trace; there is no need to hunt for captures just to bind them. Explicit immediate binding is still supported: traffic_refs or evidence_hint_id may submit already verified references, the latter reading the structured references of a specified hint in this task; if any of them is invalid, the whole report fails. TCP / no-capture cases do not need these optional parameters. The returned finding_id and finding_node_id are the standalone record and the exploration node respectively."
		case "add_hint", "add_task_hint":
			note = "\nWhen handing over a confirmed finding, keep the IDs, roles, notes and order of the verified traffic in the corresponding hint's traffic_refs (at the top level for a single one, in the matching hints element for a batch), and state in text which specific vulnerability it proves. The caller must not hand over text alone and discard existing traffic references. An unverified candidate must not be passed along as evidence."
		case "get_finding_traffic", "bind_finding_traffic", "list_findings", "list_task_findings", "node_detail", "get_task_node_detail", "update_finding_report":
			note = findingIDGuidance
		}
		if note != "" {
			out[i] = DecorateTool(tool, tool.Description()+note, tool.InputSchema())
		}
	}
	guidance := ""
	if has["report_finding"] || has["add_task_hint"] || has["add_hint"] {
		guidance = "\n\n**Traffic evidence handover (optional)**: automatic binding is done by default by the report agent after the finding is stored and before the report is written. The reporter should keep the verification commands, the key output and the IDs of existing real traffic with their roles in evidence, and pass intent_id inside a task so the report agent can trace it; there is no need to hunt for captures just to bind them. When Auto / the planner reports on someone's behalf, do not discard the references the executor already has. add_hint / add_task_hint can hand them over with traffic_refs; explicit immediate binding is still supported through report_finding's traffic_refs / evidence_hint_id. For TCP or no capture, register it normally, never guess an ID, and do not re-probe just to produce a capture."
		if has["add_task_hint"] && !has["add_hint"] {
			guidance += "\nWhen a platform conversation has no task context, do not call report_finding directly; hand it over to the corresponding existing task with add_task_hint for the task agent to register, and check the result with list_task_findings."
		}
		if has["prove_goal"] || has["goal_met"] {
			guidance += "\nBefore judging a goal complete, finish reporting/handing over the evidence you already have. Do not end the task or cancel workers while the evidence handover is unfinished merely because the finding text has been registered; having no capture does not require waiting or forcing a capture."
		}
	}
	if has["update_finding_report"] && has["bind_finding_traffic"] && has["get_finding_traffic"] {
		guidance += "\n\n**Automatically linking traffic before the report (enabled)**: you are responsible for verifying and binding the traffic for the finding that triggered this run, and then writing the report. First obtain the explicit finding_id and finding_node_id from report_finding's returned JSON or from get_task_node_detail / list_task_findings. Read the finding details, the execution trace of the corresponding intent and the existing evidence list, preferring the real IDs the reporter handed over. If this verification was HTTP and the traffic tools are available, narrow the candidates with traffic_search, then verify entry by entry with traffic_get that the request/response really supports the finding; the domain and time are only for narrowing and do not prove ownership. Link the confirmed evidence in reproduction order with bind_finding_traffic(finding_id, traffic_refs), choosing baseline / proof / verification / supporting and explaining its role. Operate only on this finding; do not create the finding again or re-probe the target. After a successful binding, call get_finding_traffic again for the latest version, read the bodies you need, and pass the version you actually read as evidence_version to update_finding_report (whose finding_id parameter still takes finding_node_id). Existing bindings need not be appended again. For TCP, nothing captured, an unavailable tool or no exact match, skip the automatic binding, write the report normally from the text/command evidence and state the reason, and never guess in order to make up the traffic. Do not claim a binding succeeded when it failed; keep the existing evidence and explain in the report why nothing was bound."
	}
	if guidance != "" || has["get_finding_traffic"] || has["update_finding_report"] {
		guidance += findingIDGuidance
	}
	return stripReachesIfDisabled(out), guidance
}

// stripReachesIfDisabled removes report_finding's "reaches" parameter when
// reachability_edges_enabled is off. Independent of findingTrafficBindingEnabled
// above -- the two settings are configured and checked separately, even though
// Task 3's handler additionally requires traffic binding to be on before a
// reaches claim can carry evidence.
func stripReachesIfDisabled(tools []actool.CoreTool) []actool.CoreTool {
	if reachabilityEdgesEnabled() {
		return tools
	}
	out := make([]actool.CoreTool, len(tools))
	for i, tool := range tools {
		if tool.Name() != "report_finding" {
			out[i] = tool
			continue
		}
		raw, _ := json.Marshal(tool.InputSchema())
		var schema map[string]any
		if json.Unmarshal(raw, &schema) == nil {
			if props, ok := schema["properties"].(map[string]any); ok {
				delete(props, "reaches")
			}
			tool = DecorateTool(tool, tool.Description(), schema)
		}
		out[i] = tool
	}
	return out
}

func stripTrafficParameters(schema map[string]any) {
	props, _ := schema["properties"].(map[string]any)
	delete(props, "traffic_refs")
	delete(props, "evidence_hint_id")
	if required, ok := schema["required"].([]any); ok {
		kept := required[:0]
		for _, key := range required {
			if key != "traffic_refs" && key != "evidence_hint_id" {
				kept = append(kept, key)
			}
		}
		schema["required"] = kept
	}
	if hints, ok := props["hints"].(map[string]any); ok {
		if items, ok := hints["items"].(map[string]any); ok {
			stripTrafficParameters(items)
		}
	}
}

// HintTrafficSchema is shared by the task-local and cross-task hint tools.
func HintTrafficSchema() map[string]any {
	return map[string]any{"type": "array", "description": "optional: verified traffic references corresponding to a specific finding in this hint, in order; after the handover, report_finding can pass evidence_hint_id to carry them.", "items": obj(map[string]any{"traffic_id": str("the real traffic ID"), "role": str("baseline / proof / verification / supporting"), "note": str("which conclusion this traffic supports")}, "traffic_id")}
}

func (t *ToolSet) findingRefsFromHint(hintID int64, explicit []db.TrafficRef) ([]db.TrafficRef, error) {
	if hintID <= 0 {
		return db.NormalizeTrafficRefs(explicit)
	}
	n, err := t.ts.GetNode(hintID) // local store only: inherited hints cannot supply evidence
	if err != nil {
		return nil, err
	}
	if n == nil || n.Kind != db.KindHint {
		return nil, fmt.Errorf("evidence_hint_id=%d must be a hint node of this task (an inherited hint cannot be used for binding directly)", hintID)
	}
	var payload struct {
		Refs []db.TrafficRef `json:"traffic_refs"`
	}
	if err := json.Unmarshal(n.Payload, &payload); err != nil {
		return nil, err
	}
	return db.NormalizeTrafficRefs(append(append([]db.TrafficRef{}, explicit...), payload.Refs...))
}
