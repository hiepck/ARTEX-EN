package server

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"sync"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

// errScriptExhausted is returned once a scriptedProvider's canned response
// sequence has been fully consumed.
var errScriptExhausted = errors.New("scriptedProvider: script exhausted")

// scriptedProvider is a programmatic fake llm.Provider for integration tests:
// it returns a pre-programmed sequence of llm.Message responses, one per
// Complete/Stream call, instead of calling a real model. It is used by the
// harness wired in wireHarnessEngine (see harness_fixture_test.go) with
// Planner/Worker forced into non-streaming mode (SetNonStreaming), so Complete
// is the path actually exercised; Stream exists only to satisfy the
// llm.Provider interface.
type scriptedProvider struct {
	mu        sync.Mutex
	responses []llm.Message
	next      int
}

// newScriptedProvider returns a *scriptedProvider that yields responses in
// order, one per Complete/Stream call, then errScriptExhausted thereafter.
func newScriptedProvider(responses ...llm.Message) *scriptedProvider {
	return &scriptedProvider{responses: responses}
}

func (p *scriptedProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.next >= len(p.responses) {
		return llm.Message{}, "", llm.Usage{}, errScriptExhausted
	}
	msg := p.responses[p.next]
	p.next++
	stopReason := "end_turn"
	if len(msg.ToolUses()) > 0 {
		stopReason = "tool_use"
	}
	return msg, stopReason, llm.Usage{}, nil
}

// Stream is not exercised by the harness (Planner/Worker are forced into
// non-streaming mode so Complete is the active path) but must satisfy
// llm.Provider. It degrades the scripted message to a single text event.
func (p *scriptedProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		msg, stopReason, _, err := p.Complete(ctx, req)
		if err != nil {
			yield(llm.StreamEvent{}, err)
			return
		}
		if !yield(llm.StreamEvent{Type: llm.SETextDelta, Text: msg.Text()}, nil) {
			return
		}
		yield(llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: stopReason}, nil)
	}
}

func TestScriptedProviderReturnsResponsesInOrder(t *testing.T) {
	m1 := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("first scripted turn")}}
	m2 := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("second scripted turn")}}
	p := newScriptedProvider(m1, m2)

	got1, _, _, err := p.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil || !reflect.DeepEqual(got1, m1) {
		t.Fatalf("call 1: got %+v, err %v; want %+v, nil", got1, err, m1)
	}
	got2, _, _, err := p.Complete(context.Background(), llm.CompletionRequest{})
	if err != nil || !reflect.DeepEqual(got2, m2) {
		t.Fatalf("call 2: got %+v, err %v; want %+v, nil", got2, err, m2)
	}
	if _, _, _, err := p.Complete(context.Background(), llm.CompletionRequest{}); !errors.Is(err, errScriptExhausted) {
		t.Fatalf("call 3: got err %v, want errScriptExhausted", err)
	}
}
