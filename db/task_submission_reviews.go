package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const TaskSubmissionReviewAgentKey = "triager"

var ErrReviewNotRunning = errors.New("this review has finished or has not started; start a new one")

// TaskSubmissionReview is an immutable historical review once its conversation turn ends.
type TaskSubmissionReview struct {
	ID             int64      `json:"id"`
	TaskID         int64      `json:"task_id"`
	ConversationID *int64     `json:"conversation_id"`
	Status         string     `json:"status"`
	Notes          string     `json:"notes"`
	Summary        string     `json:"summary"`
	Error          string     `json:"error"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

const reviewCols = `id, task_id, conversation_id, status, notes, summary, error, created_at, started_at, finished_at`

// ActiveTaskSubmissionReview is the small status payload polled by the findings list.
type ActiveTaskSubmissionReview struct {
	ID             int64  `json:"id"`
	TaskID         int64  `json:"task_id,string"`
	ConversationID int64  `json:"conversation_id"`
	Status         string `json:"status"`
}

func (d *DB) ListActiveTaskSubmissionReviews(ctx context.Context) ([]ActiveTaskSubmissionReview, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, task_id, conversation_id, status FROM task_submission_reviews
	WHERE status IN ('pending','running') AND conversation_id IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ActiveTaskSubmissionReview{}
	for rows.Next() {
		var item ActiveTaskSubmissionReview
		if err := rows.Scan(&item.ID, &item.TaskID, &item.ConversationID, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanReview(row interface{ Scan(...any) error }) (*TaskSubmissionReview, error) {
	r := &TaskSubmissionReview{}
	err := row.Scan(&r.ID, &r.TaskID, &r.ConversationID, &r.Status, &r.Notes,
		&r.Summary, &r.Error, &r.CreatedAt, &r.StartedAt, &r.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// CreateTaskSubmissionReview atomically creates the review's conversation and
// persists the first message. A task row lock deduplicates simultaneous clicks
// across clients; an existing active run is returned without dispatching.
func (d *DB) CreateTaskSubmissionReview(ctx context.Context, taskID int64, notes string) (*TaskSubmissionReview, *Conversation, bool, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, false, err
	}
	defer tx.Rollback()
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT true FROM tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&exists)
	if err != nil {
		return nil, nil, false, err
	}
	r, err := scanReview(tx.QueryRowContext(ctx, `SELECT `+reviewCols+` FROM task_submission_reviews WHERE task_id=$1 AND status IN ('pending','running')`, taskID))
	if err != nil {
		return nil, nil, false, err
	}
	if r != nil {
		return r, nil, false, nil
	}
	c, err := scanConv(tx.QueryRowContext(ctx, `INSERT INTO conversations(agent_key,title) VALUES ($1,$2) RETURNING `+convCols,
		TaskSubmissionReviewAgentKey, fmt.Sprintf("submission review - task #%d", taskID)))
	if err != nil {
		return nil, nil, false, err
	}
	r, err = scanReview(tx.QueryRowContext(ctx, `INSERT INTO task_submission_reviews(task_id,conversation_id,notes) VALUES ($1,$2,$3) RETURNING `+reviewCols,
		taskID, c.ID, strings.TrimSpace(notes)))
	if err != nil {
		return nil, nil, false, err
	}
	msg := r.InitialMessage()
	_, err = tx.ExecContext(ctx, `INSERT INTO conversation_activities(conversation_id,worker,kind,summary,detail) VALUES ($1,$2,'user',$3,$4)`,
		c.ID, TaskSubmissionReviewAgentKey, fmt.Sprintf("please review task #%d's findings before submission", taskID), msg)
	if err != nil {
		return nil, nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, false, err
	}
	return r, &c, true, nil
}

func (r *TaskSubmissionReview) InitialMessage() string {
	msg := fmt.Sprintf("Please review task #%d's current findings before submission. Call list_task_findings(task_id=%d) first for the severity/evidence overview (each entry's traffic_count tells you whether real evidence is bound), then spot-check specific findings with get_finding_traffic/get_task_node_detail as needed. Finally call record_submission_review with a Markdown report covering: (1) duplicate or same-root-cause findings that should be merged before submission, (2) findings with missing or weak evidence, (3) findings at risk of being downgraded in triage because their impact is inferred rather than observed.", r.TaskID, r.TaskID)
	if r.Notes != "" {
		msg += "\n\nAdditional notes for this review:\n" + r.Notes
	}
	return msg
}

func (d *DB) ListTaskSubmissionReviews(taskID int64) ([]*TaskSubmissionReview, error) {
	rows, err := d.Query(`SELECT `+reviewCols+` FROM task_submission_reviews WHERE task_id=$1 ORDER BY id DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TaskSubmissionReview{}
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) TaskSubmissionReviewForConversation(ctx context.Context, conversationID int64) (*TaskSubmissionReview, error) {
	return scanReview(d.QueryRowContext(ctx, `SELECT `+reviewCols+` FROM task_submission_reviews WHERE conversation_id=$1`, conversationID))
}

// FailPendingTaskSubmissionReviewForConversation seals a conversation's unfinished
// review when the runner could not even load it — the review ID is unknown on that
// path, so the conversation ID is the only handle.
func (d *DB) FailPendingTaskSubmissionReviewForConversation(conversationID int64, reason string) error {
	_, err := d.Exec(`UPDATE task_submission_reviews SET status='failed', error=$2, finished_at=now()
		WHERE conversation_id=$1 AND status IN ('pending','running')`, conversationID, reason)
	return err
}

func (d *DB) StartTaskSubmissionReview(ctx context.Context, id int64) (bool, error) {
	res, err := d.ExecContext(ctx, `UPDATE task_submission_reviews SET status='running', started_at=now() WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// RecordTaskSubmissionReviewResult never accepts a task ID: ownership comes from
// the runtime conversation. While running, each call OVERWRITES summary (a
// checkpoint) -- unlike finding_retests' write-once verdict, a long review
// session should not lose its whole analysis to an interruption partway through.
func (d *DB) RecordTaskSubmissionReviewResult(ctx context.Context, conversationID int64, summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return errors.New("summary must not be empty")
	}
	if len(summary) > 128000 {
		return errors.New("the review is too long (summary <= 128KB)")
	}
	res, err := d.ExecContext(ctx, `UPDATE task_submission_reviews SET summary=$2 WHERE conversation_id=$1 AND status='running'`, conversationID, summary)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrReviewNotRunning
	}
	return nil
}

// FinishTaskSubmissionReview seals the result. No companion-table side effect
// (unlike FinishFindingRetest): this review never mutates findings.
func (d *DB) FinishTaskSubmissionReview(id int64, status, reason string) error {
	if status != "completed" && status != "failed" && status != "stopped" {
		return errors.New("invalid terminal review status")
	}
	_, err := d.Exec(`UPDATE task_submission_reviews SET
	status=CASE WHEN $2='completed' AND summary='' THEN 'failed' ELSE $2 END,
	error=CASE WHEN $2='completed' AND summary='' THEN 'the agent saved no review; check the session and try again' ELSE $3 END,
	finished_at=now() WHERE id=$1 AND status IN ('pending','running')`, id, status, reason)
	return err
}

func (d *DB) RecoverTaskSubmissionReviews() error {
	_, err := d.Exec(`UPDATE task_submission_reviews SET status='stopped', error='the service restarted and the review was interrupted; please start it again', finished_at=now() WHERE status IN ('pending','running')`)
	return err
}
