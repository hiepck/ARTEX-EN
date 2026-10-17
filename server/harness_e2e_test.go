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
