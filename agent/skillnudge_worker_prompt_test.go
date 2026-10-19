package agent

import (
	"strings"
	"testing"
)

func TestRenderWorkerGraphOverviewMentionsUnusedRelevantSkills(t *testing.T) {
	data := map[string]any{"unused_relevant_skills": []map[string]any{{"skill": "hunt-idor", "matched_terms": []string{"idor"}}}}
	out := renderWorkerGraphOverview(data)
	if !strings.Contains(out, "unused_relevant_skills") {
		t.Fatalf("worker overview does not include the unused_relevant_skills data: %s", out)
	}
	if !strings.Contains(out, "Skill tool") {
		t.Fatalf("worker overview prose does not mention the Skill tool: %s", out)
	}
}
