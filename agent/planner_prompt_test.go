package agent

import (
	"strings"
	"testing"
)

func TestPlannerPromptDocumentsBurnSignal(t *testing.T) {
	if !strings.Contains(plannerDefaultTmpl, "burn") {
		t.Fatal("plannerDefaultTmpl does not mention the burn field")
	}
	if !strings.Contains(plannerDefaultTmpl, "kill_work") || !strings.Contains(plannerDefaultTmpl, "steer_work") {
		t.Fatal("plannerDefaultTmpl's burn guidance does not connect it to kill_work/steer_work")
	}
}

func TestPlannerPromptDocumentsAssetReachability(t *testing.T) {
	if !strings.Contains(plannerDefaultTmpl, "asset_reachability") {
		t.Fatal("plannerDefaultTmpl does not mention asset_reachability")
	}
}
