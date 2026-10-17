package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/llm"
)

// toolUseMsg builds a one-tool-call assistant turn.
func toolUseMsg(id, name string, input map[string]any) llm.Message {
	b, _ := json.Marshal(input)
	return llm.Message{
		Role: llm.RoleAssistant,
		Content: []llm.ContentBlock{{
			Type:  llm.BlockToolUse,
			ID:    id,
			Name:  name,
			Input: json.RawMessage(b),
		}},
	}
}

// textMsg builds a plain end-turn assistant message (no tool call), which
// ends an agentcore tool-use loop naturally.
func textMsg(s string) llm.Message {
	return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock(s)}}
}

// TestWireHarnessEngineStopTaskJoinsGoroutines pins down the exact mechanism
// TestEngineDrivesOnePlannerRoundThroughWorkerDispatch's cleanup relies on:
// a bare context cancel signals plannerLoop/workerLoop to stop but does not
// wait for them, so the Engine's internal runtime bookkeeping for the task
// can still show it as tracked (goroutines possibly still finishing their
// current turn) right after cancel returns. engine.StopTask, by contrast,
// cancels AND waits (rt.wg.Wait(), server/engine.go) before removing the
// runtime entry. A harness that tears down via bare cancel (then deletes the
// task / closes the DB) risks a write-after-delete race against a shared
// Postgres; StopTask closes that gap.
func TestWireHarnessEngineStopTaskJoinsGoroutines(t *testing.T) {
	m, task, _ := setupHarness(t)
	plannerProv := newScriptedProvider(textMsg("no-op"))
	workerProv := newScriptedProvider()
	engine := wireHarnessEngine(t, m, plannerProv, workerProv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go engine.Run(ctx, task)
	time.Sleep(200 * time.Millisecond) // let plannerLoop/workerLoop actually start

	engine.StopTask(task.ID)

	engine.runtimeMu.Lock()
	_, stillTracked := engine.runtimes[task.ID]
	engine.runtimeMu.Unlock()
	if stillTracked {
		t.Fatalf("StopTask returned but runtime for task %s is still tracked — goroutines did not join", task.ID)
	}
}

func TestEngineDrivesOnePlannerRoundThroughWorkerDispatch(t *testing.T) {
	m, task, target := setupHarness(t)

	plannerProv := newScriptedProvider(
		toolUseMsg("it_1", "add_intent", map[string]any{"summary": "harness probe: fetch the fake target"}),
		textMsg("intent dispatched"),
	)
	workerProv := newScriptedProvider(
		toolUseMsg("wf_1", "WebFetch", map[string]any{"url": target.URL}),
		toolUseMsg("rf_1", "record_fact", map[string]any{"summary": "harness-fact-marker: fetched fake target successfully"}),
		textMsg("done"),
	)

	engine := wireHarnessEngine(t, m, plannerProv, workerProv)
	// Join plannerLoop/workerLoop before setupHarness's own t.Cleanup deletes
	// the task / closes the DB (t.Cleanup runs LIFO, so registering this AFTER
	// setupHarness's cleanups makes it run FIRST). A bare context cancel only
	// signals the loops to stop; it does not wait, so a goroutine mid-turn
	// could still write to Postgres after DeleteTask/Close — see
	// TestWireHarnessEngineStopTaskJoinsGoroutines, which pins down why
	// StopTask (cancel + join) is required instead.
	t.Cleanup(func() { engine.StopTask(task.ID) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go engine.Run(ctx, task)

	deadline := time.Now().Add(25 * time.Second)
	var lastState string
	for time.Now().Before(deadline) {
		facts, err := task.Store.ListByKind(db.KindFact, 10)
		n := 0
		if err == nil {
			n = len(facts)
			for _, f := range facts {
				if strings.Contains(string(f.Payload), "harness-fact-marker") {
					return // success
				}
			}
		}
		lastState = fmt.Sprintf("facts=%d err=%v", n, err)
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for scripted fact; last observed state: %s", lastState)
}
