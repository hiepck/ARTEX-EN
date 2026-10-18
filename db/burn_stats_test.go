package db

import (
	"testing"
	"time"
)

func TestBurnStatsByIntentAggregatesAndIsolates(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) - skipping", err)
	}
	defer d.Close()

	expID, err := d.CreateExploration("burn stats", "aggregate per-intent burn")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM explorations WHERE id=$1`, expID)
	store := d.Exploration(expID)

	busyID, err := store.AddIntent(map[string]any{"summary": "busy intent"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	idleID, err := store.AddIntent(map[string]any{"summary": "idle intent, no activity"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}

	otherExpID, err := d.CreateExploration("burn stats other", "foreign intent must not match")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Exec(`DELETE FROM explorations WHERE id=$1`, otherExpID)
	foreignID, err := d.Exploration(otherExpID).AddIntent(map[string]any{"summary": "foreign"}, 1, nil, "planner")
	if err != nil {
		t.Fatal(err)
	}
	// Give the foreign intent its own activity so a leak would be visible, not just absent-by-coincidence.
	if _, err := d.Exploration(otherExpID).AppendActivity(Activity{NodeID: &foreignID, Worker: "work#1", Kind: "tool_use"}); err != nil {
		t.Fatal(err)
	}

	in, out := 100, 50
	appendToolUse := func() {
		t.Helper()
		if _, err := store.AppendActivity(Activity{NodeID: &busyID, Worker: "work#1", Kind: "tool_use"}); err != nil {
			t.Fatal(err)
		}
	}
	appendToolUse()
	appendToolUse()
	if _, err := store.AppendActivity(Activity{NodeID: &busyID, Worker: "work#1", Kind: "result", InputTokens: &in, OutputTokens: &out}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // give ElapsedSeconds something nonzero to observe

	got, err := store.BurnStatsByIntent([]int64{busyID, idleID, foreignID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want exactly 1 (busy only): %+v", len(got), got)
	}
	b, ok := got[busyID]
	if !ok {
		t.Fatalf("busy intent missing from result: %+v", got)
	}
	// ToolCalls counts kind IN ('tool_use','tool_result') rows only -- the
	// terminal 'result' row is the turn's summary, not a tool invocation.
	if b.ToolCalls != 2 {
		t.Fatalf("ToolCalls = %d, want 2", b.ToolCalls)
	}
	if b.InputTokens != 100 || b.OutputTokens != 50 {
		t.Fatalf("tokens = %d/%d, want 100/50", b.InputTokens, b.OutputTokens)
	}
	if b.ElapsedSeconds < 1.0 {
		t.Fatalf("ElapsedSeconds = %v, want >= 1.0 after the sleep", b.ElapsedSeconds)
	}
	if _, ok := got[idleID]; ok {
		t.Fatalf("idle intent (no activity) must be absent, not a zero-value entry: %+v", got[idleID])
	}
	if _, ok := got[foreignID]; ok {
		t.Fatalf("foreign-exploration intent must never appear: %+v", got[foreignID])
	}

	empty, err := store.BurnStatsByIntent(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("BurnStatsByIntent(nil) = %+v, %v; want empty map, nil error", empty, err)
	}
}
