package db

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func reviewDB(t *testing.T) (*DB, int64) {
	t.Helper()
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	tk, err := d.CreateTask("review-test", "test the review feature", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(`DELETE FROM conversations WHERE id IN (SELECT conversation_id FROM task_submission_reviews WHERE task_id=$1)`, tk.ID)
		_ = d.DeleteTask(tk.ID) // cascades task_submission_reviews via ON DELETE CASCADE
		d.Close()
	})
	return d, tk.ID
}

func TestReviewAtomicDeduplicationAndHistory(t *testing.T) {
	d, taskID := reviewDB(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	createdCount := 0
	ids := make([]int64, 0, 8)
	for range 8 {
		wg.Go(func() {
			r, c, created, err := d.CreateTaskSubmissionReview(t.Context(), taskID, "focus on SSRF")
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			ids = append(ids, r.ID)
			if created {
				createdCount++
				if c == nil {
					t.Error("created without conversation")
				}
			}
		})
	}
	wg.Wait()
	if createdCount != 1 || len(ids) != 8 {
		t.Fatalf("created=%d ids=%v", createdCount, ids)
	}
	for _, id := range ids {
		if id != ids[0] {
			t.Fatal("duplicate active reviews", ids)
		}
	}
	rows, err := d.ListTaskSubmissionReviews(taskID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("history=%v err=%v", rows, err)
	}
	r := rows[0]
	ctx := t.Context()
	if err := d.RecordTaskSubmissionReviewResult(ctx, *r.ConversationID, "draft"); !errors.Is(err, ErrReviewNotRunning) {
		t.Fatal("pending accepted result", err)
	}
	if ok, err := d.StartTaskSubmissionReview(ctx, r.ID); err != nil || !ok {
		t.Fatalf("start=%t %v", ok, err)
	}
	if err := d.RecordTaskSubmissionReviewResult(ctx, 0, "x"); !errors.Is(err, ErrReviewNotRunning) {
		t.Fatal("unscoped write accepted", err)
	}
	// Checkpoint semantics: two different calls while running both succeed, second wins.
	if err := d.RecordTaskSubmissionReviewResult(ctx, *r.ConversationID, "first pass: 3 findings reviewed"); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordTaskSubmissionReviewResult(ctx, *r.ConversationID, "final: 26 findings, 10 duplicate groups"); err != nil {
		t.Fatal(err)
	}
	if err := d.FinishTaskSubmissionReview(r.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.RecordTaskSubmissionReviewResult(ctx, *r.ConversationID, "too late"); !errors.Is(err, ErrReviewNotRunning) {
		t.Fatal("overwrote sealed result", err)
	}
	rows, _ = d.ListTaskSubmissionReviews(taskID)
	if len(rows) != 1 || rows[0].Status != "completed" || rows[0].Summary != "final: 26 findings, 10 duplicate groups" {
		t.Fatal("checkpoint overwrite did not win", rows)
	}

	// A second review after the first finishes is new history, and a review that
	// finishes with no summary at all must be marked failed.
	next, _, created, err := d.CreateTaskSubmissionReview(ctx, taskID, "")
	if err != nil || !created || next.ID == r.ID {
		t.Fatalf("new history=%+v created=%t err=%v", next, created, err)
	}
	if err := d.FinishTaskSubmissionReview(next.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	rows, _ = d.ListTaskSubmissionReviews(taskID)
	if len(rows) != 2 || rows[0].Status != "failed" || rows[0].Error == "" {
		t.Fatal("missing summary treated as successful", rows)
	}
}

func TestReviewDeletionAndRestart(t *testing.T) {
	d, taskID := reviewDB(t)
	r, c, _, err := d.CreateTaskSubmissionReview(t.Context(), taskID, "delete")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteConversation(c.ID); err != nil {
		t.Fatal(err)
	}
	rows, _ := d.ListTaskSubmissionReviews(taskID)
	if len(rows) != 1 || rows[0].ConversationID != nil || rows[0].Status != "stopped" {
		t.Fatal(rows)
	}
	next, c2, created, err := d.CreateTaskSubmissionReview(t.Context(), taskID, "restart")
	if err != nil || !created {
		t.Fatal("deleted session blocks retry", err)
	}
	if ok, err := d.StartTaskSubmissionReview(t.Context(), next.ID); err != nil || !ok {
		t.Fatal(err)
	}
	if err := d.RecoverTaskSubmissionReviews(); err != nil {
		t.Fatal(err)
	}
	if err := d.FinishTaskSubmissionReview(next.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	full, _ := d.TaskSubmissionReviewForConversation(t.Context(), c2.ID)
	if full.Status != "stopped" || full.FinishedAt == nil {
		t.Fatal("restart result overwritten", full)
	}
	_ = d.DeleteConversation(c2.ID)
	_, _ = d.Exec(`DELETE FROM task_submission_reviews WHERE id IN ($1,$2)`, r.ID, next.ID)
}

func TestReviewValidation(t *testing.T) {
	d, taskID := reviewDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, _, err := d.CreateTaskSubmissionReview(ctx, taskID, ""); err == nil {
		t.Fatal("cancelled creation succeeded")
	}
	for _, s := range []string{"", strings.Repeat("x", 128001)} {
		if err := d.RecordTaskSubmissionReviewResult(t.Context(), 0, s); err == nil {
			t.Fatal("bad result accepted, len", len(s))
		}
	}
	if _, _, _, err := d.CreateTaskSubmissionReview(t.Context(), 999999999, ""); err == nil {
		t.Fatal("review created for a nonexistent task")
	}
}
