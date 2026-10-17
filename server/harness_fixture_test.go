package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
