package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	actool "github.com/Autumn-27/norma/tool"
)

func (s *Server) listActiveTaskSubmissionReviews(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	items, err := pg.ListActiveTaskSubmissionReviews(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"reviews": items})
}

func (s *Server) listTaskSubmissionReviews(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	taskID, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	items, err := pg.ListTaskSubmissionReviews(taskID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"reviews": items})
}

func (s *Server) startTaskSubmissionReview(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	taskID, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var req struct {
		Notes string `json:"notes"`
	}
	if !decodeConversationRequest(w, r, &req) {
		return
	}
	req.Notes = strings.TrimSpace(req.Notes)
	if utf8.RuneCountInString(req.Notes) > 4000 {
		writeErr(w, 400, "the review notes may be at most 4000 characters")
		return
	}
	a, err := pg.GetAgentByKey(db.TaskSubmissionReviewAgentKey)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if a == nil || !a.Enabled {
		writeErr(w, 409, "the submission review agent does not exist or is not enabled; configure triager under agent management")
		return
	}
	for _, key := range []string{"record_submission_review"} {
		tl, err := pg.GetTool(key)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if tl == nil || !tl.Enabled || !slices.Contains(tl.Agents, a.Key) {
			writeErr(w, 409, "enable and bind this tool for the review agent: "+key)
			return
		}
	}
	if s.resolveChatAgent(&db.Conversation{AgentKey: a.Key}) == nil {
		writeErr(w, 503, s.chatUnavailableReason())
		return
	}
	if s.ctx.Err() != nil {
		writeErr(w, 503, "the service is shutting down")
		return
	}
	review, conv, created, err := pg.CreateTaskSubmissionReview(r.Context(), taskID, req.Notes)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "task not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if created {
		busyKey := s.convBusyKey(conv.ID)
		s.chatMu.Lock()
		s.chatBusy[busyKey] = true
		s.chatMu.Unlock()
		s.runConversation(conv, review.InitialMessage(), busyKey)
	}
	code := http.StatusOK
	if created {
		code = http.StatusAccepted
	}
	writeJSON(w, code, map[string]any{"review": review, "created": created})
}

// The tool derives the review from server-owned conversation context. Tool
// arguments cannot redirect a result into a different review's conversation.
func (s *Server) taskSubmissionReviewTools() []actool.CoreTool {
	return []actool.CoreTool{
		wrTool("record_submission_review", "Save this review session's report. Overwrites any earlier checkpoint from this same session; only the last call before the session ends is kept. The original findings are never modified.",
			objSchema(map[string]any{
				"summary": strParam("the review report, Markdown: duplicate/merge candidates, evidence gaps, severity-downgrade risk"),
			}, "summary"), func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
				var a struct {
					Summary string `json:"summary"`
				}
				if err := json.Unmarshal(in, &a); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				if err := s.m.pg.RecordTaskSubmissionReviewResult(ctx, intercept.ConvIDFromContext(ctx), a.Summary); err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return jsonResult(map[string]any{"saved": true})
			}),
	}
}

// The advisory lock key is unique within this repo (distinct from retester's 7337741010).
const taskSubmissionReviewSeedLockKey = 7337741020

// Seed the editable agent atomically, without an automatic discovery trigger.
// Once seeded, user edits/deletion survive restarts; a pre-existing key is kept.
func (s *Server) seedTaskSubmissionReviewer() error {
	for _, t := range s.taskSubmissionReviewTools() {
		schema, _ := json.Marshal(t.InputSchema())
		bindings, _ := json.Marshal([]string{db.TaskSubmissionReviewAgentKey})
		if err := s.m.pg.SeedTool(t.Name(), t.Description(), schema, bindings); err != nil {
			return err
		}
	}
	const flag = "task_submission_review_seed_v1"
	tx, err := s.m.pg.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Lock this migration, including concurrent server initialization.
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(` + strconv.Itoa(taskSubmissionReviewSeedLockKey) + `)`); err != nil {
		return err
	}
	var done string
	err = tx.QueryRow(`SELECT value FROM settings WHERE key=$1`, flag).Scan(&done)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if done != "true" {
		var id int64
		err = tx.QueryRow(`INSERT INTO agents(key,name,description,role,builtin,enabled)
		VALUES ($1,'Submission review','Started by hand before submitting findings; reviews a task''s current findings for duplicates, evidence gaps, and severity risk.','assistant',false,true)
		ON CONFLICT (key) DO NOTHING RETURNING id`, db.TaskSubmissionReviewAgentKey).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if id > 0 {
			var pid int64
			if err = tx.QueryRow(`INSERT INTO agent_prompts(agent_id,version,template_text,note,updated_by)
			VALUES ($1,1,$2,'builtin default','system') RETURNING id`, id, agent.TriagerDefaultPrompt).Scan(&pid); err != nil {
				return err
			}
			if _, err = tx.Exec(`UPDATE agents SET current_prompt_id=$1 WHERE id=$2`, pid, id); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`INSERT INTO settings(key,value) VALUES ($1,'true') ON CONFLICT(key) DO UPDATE SET value='true'`, flag); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	// Additive, idempotent: SeedTool above would no-op on these 7 pre-existing
	// tools (ON CONFLICT DO NOTHING), so bind them here instead.
	return s.m.pg.AddAgentToToolBinding(db.TaskSubmissionReviewAgentKey, []string{
		"list_task_findings", "get_task_node_detail", "get_task_worker_trace",
		"list_task_worker_traces", "search_task_worker_traces", "get_task_graph", "get_finding_traffic",
	})
}

func (s *Server) finishTaskSubmissionReview(id int64, status, reason string) {
	if err := s.m.pg.FinishTaskSubmissionReview(id, status, reason); err != nil {
		log.Printf("[triager] finish: %v", err)
	}
}
