package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
)

// TestHTTPRequestToolOptInAndWorksEndToEnd proves the opt-in mechanism for
// real: it binds http_request to the worker via AddAgentToToolBinding and
// then confirms the tool actually reached the network (hits >= 1), not just
// that the scripted worker's record_fact fired. scriptedProvider plays back
// its script regardless of whether an intervening tool call succeeded, so a
// test that only checked for the resulting fact would pass even if
// http_request were never wired into the worker's tool list at all.
func TestHTTPRequestToolOptInAndWorksEndToEnd(t *testing.T) {
	m, task, _ := setupHarness(t)
	wireTools(m.pg, nil)
	t.Cleanup(func() { agent.ToolResolve = nil })
	if err := m.pg.AddAgentToToolBinding("worker", []string{"http_request"}); err != nil {
		t.Fatal(err)
	}

	var hits int32
	httpTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = io.WriteString(w, "fake-target-ok")
	}))
	defer httpTarget.Close()

	plannerProv := newScriptedProvider(
		toolUseMsg("it_1", "add_intent", map[string]any{"summary": "harness probe: use http_request against the fake target"}),
		textMsg("intent dispatched"),
	)
	workerProv := newScriptedProvider(
		toolUseMsg("hr_1", "http_request", map[string]any{"url": httpTarget.URL}),
		toolUseMsg("rf_1", "record_fact", map[string]any{"summary": "harness-http-tool-marker: reached the fake target via http_request"}),
		textMsg("done"),
	)

	engine := wireHarnessEngine(t, m, plannerProv, workerProv)
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
				if strings.Contains(string(f.Payload), "harness-http-tool-marker") {
					if atomic.LoadInt32(&hits) < 1 {
						t.Fatalf("record_fact fired but http_request never reached the fake target (hits=%d) — the tool call was likely rejected as unknown", hits)
					}
					return
				}
			}
		}
		lastState = fmt.Sprintf("facts=%d hits=%d err=%v", n, atomic.LoadInt32(&hits), err)
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for scripted fact; last observed state: %s", lastState)
}
