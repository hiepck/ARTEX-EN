package server

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/llm"
	actool "github.com/Autumn-27/norma/tool"
)

type reviewProvider struct {
	complete func(context.Context, llm.CompletionRequest) (llm.Message, string, llm.Usage, error)
}

func (p reviewProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	return p.complete(ctx, req)
}
func (p reviewProvider) Stream(context.Context, llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		yield(llm.StreamEvent{}, errors.New("test expects non-streaming"))
	}
}

func newReviewServer(t *testing.T) (*Server, int64) {
	t.Helper()
	dsn, _, err := db.DSN()
	if err != nil {
		t.Skip("test postgres not configured")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	td := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{ctx: ctx, m: &Manager{pg: pg, dir: td, tasks: map[string]*Task{}},
		chatBusy: map[string]bool{}, chatCancel: map[string]context.CancelCauseFunc{}}
	if err := s.seedTaskSubmissionReviewer(); err != nil {
		t.Fatal(err)
	}
	task, err := s.m.CreateTask("review-server", "test the review feature", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := strconv.ParseInt(task.ID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	oldAugment, oldResolve, oldPrompt := agent.ToolAugment, agent.ToolResolve, agent.PromptOverride
	agent.ToolAugment = func(context.Context, string) ([]actool.CoreTool, agent.DeferredInfo, func()) {
		return s.taskSubmissionReviewTools(), agent.DeferredInfo{}, func() {}
	}
	agent.ToolResolve = nil
	agent.PromptOverride = func(string) (string, bool) { return agent.TriagerDefaultPrompt, true }
	t.Cleanup(func() {
		cancel()
		waitReviewIdle(t, s)
		agent.ToolAugment, agent.ToolResolve, agent.PromptOverride = oldAugment, oldResolve, oldPrompt
		_, _ = pg.Exec(`DELETE FROM conversations WHERE id IN (SELECT conversation_id FROM task_submission_reviews WHERE task_id=$1)`, taskID)
		_ = pg.DeleteTask(taskID)
		pg.Close()
	})
	return s, taskID
}

func setReviewProvider(s *Server, p reviewProvider) {
	s.chatAgent = agent.NewChatAgent(p, "test", s.m.dir, nil, 10000)
	s.chatAgent.SetNonStreaming(func() bool { return true })
}

func reviewRequest(handler http.HandlerFunc, method string, id int64, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/", strings.NewReader(body))
	r.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	handler(w, r)
	return w
}

func waitReviewIdle(t *testing.T, s *Server) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.chatMu.Lock()
		n := len(s.chatBusy)
		s.chatMu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("conversation did not become idle")
}

func TestReviewActiveStatusLifecycle(t *testing.T) {
	s, taskID := newReviewServer(t)
	check := func(want string, conversationID int64) {
		t.Helper()
		w := reviewRequest(s.listActiveTaskSubmissionReviews, http.MethodGet, 0, "")
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var body struct {
			Reviews []db.ActiveTaskSubmissionReview `json:"reviews"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		var matches []db.ActiveTaskSubmissionReview
		for _, item := range body.Reviews {
			if item.TaskID == taskID {
				matches = append(matches, item)
			}
		}
		if want == "" {
			if len(matches) != 0 {
				t.Fatalf("terminal review still active: %+v", matches)
			}
		} else if len(matches) != 1 || matches[0].Status != want || matches[0].ConversationID != conversationID {
			t.Fatalf("active=%+v want status=%s conversation=%d", matches, want, conversationID)
		}
		for _, field := range []string{`"summary"`, `"notes"`} {
			if strings.Contains(w.Body.String(), field) {
				t.Fatalf("status response leaks %s", field)
			}
		}
	}
	check("", 0)
	for _, terminal := range []string{"completed", "failed", "stopped"} {
		r, c, _, err := s.m.pg.CreateTaskSubmissionReview(t.Context(), taskID, "private notes")
		if err != nil {
			t.Fatal(err)
		}
		check("pending", c.ID)
		if _, err := s.m.pg.StartTaskSubmissionReview(t.Context(), r.ID); err != nil {
			t.Fatal(err)
		}
		check("running", c.ID)
		if terminal == "completed" {
			if err := s.m.pg.RecordTaskSubmissionReviewResult(t.Context(), c.ID, "26 findings reviewed"); err != nil {
				t.Fatal(err)
			}
			check("running", c.ID) // A staged summary is still running until the turn ends.
		}
		if err := s.m.pg.FinishTaskSubmissionReview(r.ID, terminal, ""); err != nil {
			t.Fatal(err)
		}
		check("", 0)
	}
}

func TestReviewHTTPThroughConversationAndTools(t *testing.T) {
	s, taskID := newReviewServer(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	setReviewProvider(s, reviewProvider{complete: func(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
		n := calls.Add(1)
		if n == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return llm.Message{}, "", llm.Usage{}, ctx.Err()
			}
		}
		if n == 1 {
			return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.BlockToolUse, ID: "1", Name: "record_submission_review", Input: json.RawMessage(`{"summary":"no duplicates found; all findings have evidence"}`)}}}, "tool_use", llm.Usage{}, nil
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("the review was saved")}}, "end_turn", llm.Usage{}, nil
	}})
	w := reviewRequest(s.startTaskSubmissionReview, "POST", taskID, `{"notes":"focus on the SSRF findings"}`)
	if w.Code != 202 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provider not called")
	}
	dup := reviewRequest(s.startTaskSubmissionReview, "POST", taskID, `{}`)
	if dup.Code != 200 || !strings.Contains(dup.Body.String(), `"created":false`) {
		t.Fatalf("duplicate %d %s", dup.Code, dup.Body)
	}
	close(release)
	waitReviewIdle(t, s)
	rows, err := s.m.pg.ListTaskSubmissionReviews(taskID)
	if err != nil || len(rows) != 1 || rows[0].Status != "completed" || rows[0].Summary != "no duplicates found; all findings have evidence" {
		t.Fatalf("history=%+v err=%v", rows, err)
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected LLM calls", calls.Load())
	}
	listing := reviewRequest(s.listTaskSubmissionReviews, "GET", taskID, "")
	if listing.Code != 200 || !strings.Contains(listing.Body.String(), "no duplicates found") {
		t.Fatalf("history %d %s", listing.Code, listing.Body)
	}
	res, err := s.taskSubmissionReviewTools()[0].Call(context.Background(), json.RawMessage(`{"summary":"bad overwrite"}`), nil)
	if err != nil || !res.IsError {
		t.Fatal("unassociated call accepted", res, err)
	}
}

func TestReviewStopsFailuresAndNoSummary(t *testing.T) {
	for _, mode := range []string{"stop", "failure", "no_summary"} {
		t.Run(mode, func(t *testing.T) {
			s, taskID := newReviewServer(t)
			setReviewProvider(s, reviewProvider{complete: func(ctx context.Context, _ llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
				if mode == "stop" {
					<-ctx.Done()
					return llm.Message{}, "", llm.Usage{}, ctx.Err()
				}
				if mode == "failure" {
					return llm.Message{}, "", llm.Usage{}, errors.New("test model failed")
				}
				return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("no report was saved")}}, "end_turn", llm.Usage{}, nil
			}})
			w := reviewRequest(s.startTaskSubmissionReview, "POST", taskID, `{}`)
			if w.Code != 202 {
				t.Fatalf("create %d %s", w.Code, w.Body)
			}
			if mode == "stop" {
				var body struct {
					Review db.TaskSubmissionReview `json:"review"`
				}
				_ = json.Unmarshal(w.Body.Bytes(), &body)
				stop := reviewRequest(s.pgStopConversation, "POST", *body.Review.ConversationID, `{}`)
				if stop.Code != 200 || !strings.Contains(stop.Body.String(), "stopping") {
					t.Fatalf("immediate stop: %d %s", stop.Code, stop.Body)
				}
			}
			waitReviewIdle(t, s)
			rows, _ := s.m.pg.ListTaskSubmissionReviews(taskID)
			want := "failed"
			if mode == "stop" {
				want = "stopped"
			}
			if len(rows) != 1 || rows[0].Status != want || rows[0].Error == "" {
				t.Fatalf("expected %s: %+v", want, rows)
			}
		})
	}
}

func TestReviewValidationSeedingAndScope(t *testing.T) {
	s, taskID := newReviewServer(t)
	for _, tc := range []struct {
		id   int64
		body string
		code int
	}{{0, `{}`, 404}, {taskID, `{`, 400}, {taskID, `{"notes":"` + strings.Repeat("é", 4001) + `"}`, 400}, {999999999, `{}`, 404}, {taskID, `{}`, 503}} {
		w := reviewRequest(s.startTaskSubmissionReview, "POST", tc.id, tc.body)
		if w.Code != tc.code {
			t.Fatalf("code=%d want=%d %s", w.Code, tc.code, w.Body)
		}
	}
	rows, _ := s.m.pg.ListTaskSubmissionReviews(taskID)
	if len(rows) != 0 {
		t.Fatal("invalid request created state")
	}
	for _, tool := range s.taskSubmissionReviewTools() {
		res, err := tool.Call(t.Context(), json.RawMessage(`{"summary":"x"}`), nil)
		if err != nil || !res.IsError {
			t.Fatal("unassociated conversation accepted", tool.Name(), res, err)
		}
	}
	a, _ := s.m.pg.GetAgentByKey(db.TaskSubmissionReviewAgentKey)
	if a == nil || a.Builtin {
		t.Fatal("missing editable triager")
	}
	var triggers int
	if err := s.m.pg.QueryRow(`SELECT count(*) FROM agent_triggers WHERE agent_key=$1`, db.TaskSubmissionReviewAgentKey).Scan(&triggers); err != nil || triggers != 0 {
		t.Fatal("unexpected automatic trigger", triggers, err)
	}
	if _, err := s.m.pg.SavePrompt(a.ID, "customized review prompt", "test", "test"); err != nil {
		t.Fatal(err)
	}
	if err := s.seedTaskSubmissionReviewer(); err != nil {
		t.Fatal(err)
	}
	var prompt string
	if err := s.m.pg.QueryRow(`SELECT p.template_text FROM agent_prompts p JOIN agents a ON a.current_prompt_id=p.id WHERE a.key=$1`, a.Key).Scan(&prompt); err != nil || prompt != "customized review prompt" {
		t.Fatal("seed overwrote prompt", prompt, err)
	}
	_, _ = s.m.pg.SavePrompt(a.ID, agent.TriagerDefaultPrompt, "restore", "test")
	for _, key := range []string{"list_task_findings", "get_task_node_detail", "get_task_worker_trace",
		"list_task_worker_traces", "search_task_worker_traces", "get_task_graph", "get_finding_traffic"} {
		tl, err := s.m.pg.GetTool(key)
		if err != nil || tl == nil {
			t.Fatalf("pre-existing tool %s missing: %v", key, err)
		}
		found := false
		for _, ak := range tl.Agents {
			if ak == db.TaskSubmissionReviewAgentKey {
				found = true
			}
		}
		if !found {
			t.Fatalf("tool %s not bound to %s: agents=%v", key, db.TaskSubmissionReviewAgentKey, tl.Agents)
		}
	}
}
