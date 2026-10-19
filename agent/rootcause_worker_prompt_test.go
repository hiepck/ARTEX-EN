package agent

import (
	"strings"
	"testing"
)

func TestRenderWorkerGraphOverviewMentionsRootCauseClusters(t *testing.T) {
	data := map[string]any{"root_cause_clusters": []map[string]any{{"finding_ids": []int64{1, 2}}}}
	out := renderWorkerGraphOverview(data)
	if !strings.Contains(out, "shares_root_cause_with") {
		t.Fatalf("worker overview prose does not mention shares_root_cause_with: %s", out)
	}
	if !strings.Contains(out, "root_cause_clusters") {
		t.Fatalf("worker overview does not include the root_cause_clusters data: %s", out)
	}
}
