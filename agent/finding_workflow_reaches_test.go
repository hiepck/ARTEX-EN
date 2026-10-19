package agent

import (
	"encoding/json"
	"testing"

	actool "github.com/Autumn-27/norma/tool"
)

func TestFindingWorkflowStripsReachesWhenDisabled(t *testing.T) {
	oldReach, oldResolve, oldAugment := ReachabilityEdgesEnabled, ToolResolve, ToolAugment
	t.Cleanup(func() { ReachabilityEdgesEnabled, ToolResolve, ToolAugment = oldReach, oldResolve, oldAugment })
	ToolAugment = nil
	ToolResolve = nil
	on := false
	ReachabilityEdgesEnabled = func() bool { return on }
	ts := NewToolSet(nil, "fixture")

	for _, enabled := range []bool{false, true, false} {
		on = enabled
		out, _, cleanup := AugmentTools(t.Context(), "worker", []actool.CoreTool{ts.addFinding()})
		defer cleanup()
		for _, tool := range out {
			if tool.Name() != "report_finding" {
				continue
			}
			raw, _ := json.Marshal(tool.InputSchema())
			var schema map[string]any
			json.Unmarshal(raw, &schema)
			props, _ := schema["properties"].(map[string]any)
			if (props["reaches"] != nil) != on {
				t.Fatalf("enabled=%v: reaches presence in schema = %v, want %v", on, props["reaches"] != nil, on)
			}
		}
	}
}
