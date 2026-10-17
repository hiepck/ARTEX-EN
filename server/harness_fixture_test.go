package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/norma/llm"
)

// setupHarness builds the Postgres-backed fixture shared by the integration
// harness: a *Manager + one *Task (skip-tolerant when Postgres is
// unavailable, per server/core_test.go's convention) and a fake HTTP target
// the scripted worker's tool calls can hit. Both are cleaned up via
// t.Cleanup.
func setupHarness(t *testing.T) (m *Manager, task *Task, target *httptest.Server) {
	t.Helper()

	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { m.Close() })

	task, err = m.CreateTask("harness e2e", "harness goal", nil, 0, 0)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() { _, _ = m.DeleteTask(task.ID, DeleteTaskOptions{}) })

	target = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "fake-target-ok")
	}))
	t.Cleanup(target.Close)

	return m, task, target
}

// wireHarnessEngine builds one fake-provider-backed Planner/Worker and
// installs them into a real *Engine via SetAuthoritativeAgentResolver, so
// Engine.Run drives them for real instead of resolving the DB-configured
// agents. Both are forced into non-streaming mode (SetNonStreaming) so the
// loop calls scriptedProvider.Complete exclusively — see the ruling in this
// plan's ledger (Task 1): scriptedProvider's Stream degrades a tool_use
// message to a single text event and cannot drive agentcore's tool-calling
// loop on its own.
//
// No standalone test wraps this function: *Engine exposes
// SetAuthoritativeAgentResolver (server/engine.go:720) but no getter to read
// the installed resolver back, and the resolver is invoked only internally
// by plannerLoop/workerLoop during Run. Its correctness is verified end to
// end by TestEngineDrivesOnePlannerRoundThroughWorkerDispatch (Task 4).
func wireHarnessEngine(t *testing.T, m *Manager, prov llm.Provider) *Engine {
	t.Helper()

	pl := agent.NewPlanner(prov, "harness-fake-model", t.TempDir(), nil, 200000, 10)
	pl.SetNonStreaming(func() bool { return true })

	wk := agent.NewWorker(prov, "harness-fake-model", t.TempDir(), nil, 200000, 10)
	wk.SetNonStreaming(func() bool { return true })

	e := NewEngine(m)
	e.SetAuthoritativeAgentResolver(func(*Task) (*agent.Planner, *agent.Worker) {
		return pl, wk
	})
	return e
}

func TestSetupHarnessCreatesTaskAndTarget(t *testing.T) {
	m, task, target := setupHarness(t)
	if m == nil || task == nil || task.Store == nil {
		t.Fatalf("setupHarness returned incomplete fixture: m=%v task=%v store=%v", m, task, task.Store)
	}
	resp, err := http.Get(target.URL)
	if err != nil {
		t.Fatalf("fake target unreachable: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fake-target-ok" {
		t.Fatalf("fake target body = %q, want %q", body, "fake-target-ok")
	}
}
