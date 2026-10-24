package server

import (
	"context"
	"slices"
	"testing"
)

// tServerHostTools builds a real Server (skipping without PG) and returns its
// hostTools output — the same provider wired into wireAgentAugment.
func tServerHostTools(t *testing.T) ([]any, map[string][]string) {
	t.Helper()
	m, err := NewManager(t.TempDir(), "")
	if err != nil || m.pg == nil {
		t.Skipf("postgres unavailable (%v) — skipping hostTools test", err)
	}
	t.Cleanup(func() { m.Close() })
	s := New(context.Background(), m, t.TempDir(), t.TempDir(), t.TempDir())
	tools, deferred := s.hostTools()
	return toAnyTools(tools), deferred
}

// The review's record_submission_review tool must reach the running agent via the
// real wiring (hostTools → ToolAugment), not only exist as a seeded DB row. It was
// missing from hostTools once: the start-review API check passed (the tool table
// had it enabled and bound) but the conversation's tool set never contained it, so
// the triager ended with "the agent saved no review" on every real run.
func TestHostToolsExposeSubmissionReviewTools(t *testing.T) {
	tools, _ := tServerHostTools(t)
	have := toolNames(tools)
	if !slices.Contains(have, "record_submission_review") {
		t.Fatalf("hostTools must expose record_submission_review for the triager, got %v", have)
	}
}

// The retester's tools ride the same wiring; assert both families stay present so
// a future hostTools edit cannot silently drop either one again.
func TestHostToolsExposeRetestAndReviewToolFamilies(t *testing.T) {
	tools, _ := tServerHostTools(t)
	names := toolNames(tools)
	for _, want := range []string{"record_finding_retest_result", "record_submission_review"} {
		if !slices.Contains(names, want) {
			t.Fatalf("hostTools lost %q (available: %v)", want, names)
		}
	}
}

// toolNames extracts Name() from the tool interfaces hostTools returned.
func toolNames(tools []any) []string {
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		if n, ok := tl.(interface{ Name() string }); ok {
			names = append(names, n.Name())
		}
	}
	return names
}

// toAnyTools erases the CoreTool type so the helper's signature stays free of the
// actool import even when tests run without a database.
func toAnyTools[T any](in []T) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = v
	}
	return out
}
