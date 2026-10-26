package db

import (
	"strings"
	"time"
)

// GlobalKnowledgeHit is one confirmed fact or finding from ANY task, returned by a
// keyword search. It powers cross-task pattern memory: a hunt in one task can reuse
// what another hunt already learned (a bypass that worked, a dead end, a stack
// quirk) instead of re-discovering it from scratch — the per-task list_facts and
// search_all_worker_traces only ever see this task plus directly linked ones.
type GlobalKnowledgeHit struct {
	TaskID    int64     `json:"task_id"`
	NodeID    int64     `json:"node_id"`
	Kind      string    `json:"kind"` // fact | finding
	Summary   string    `json:"summary"`
	Severity  string    `json:"severity,omitempty"`
	VulnClass string    `json:"vulnclass,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SearchGlobalKnowledge searches confirmed facts and findings across every task by
// keyword (case-insensitive substring on the payload summary). The result is capped
// so a caller's context stays bounded; newest nodes win, because a recent lesson is
// the one most likely to still describe the target/stack.
//
// Deliberately keyword-based, not semantic: it needs no embedding store, works on
// every existing database without a migration, and the caller (an LLM) is good at
// choosing search terms. A hit from another task is a hint, never authority — the
// caller must verify it against the current target.
func (d *DB) SearchGlobalKnowledge(q string, limit int) ([]GlobalKnowledgeHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := d.Query(`
SELECT t.id, n.id, n.kind, COALESCE(n.payload->>'summary',''),
       COALESCE(n.payload->>'severity',''), COALESCE(n.payload->>'vulnclass',''), n.updated_at
FROM exploration_nodes n
JOIN tasks t ON t.exploration_id = n.exploration_id
WHERE n.kind IN ('fact','finding') AND n.state = 'confirmed'
  AND n.payload->>'summary' ILIKE '%'||$1||'%'
ORDER BY n.id DESC
LIMIT $2`, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GlobalKnowledgeHit
	for rows.Next() {
		var h GlobalKnowledgeHit
		if err := rows.Scan(&h.TaskID, &h.NodeID, &h.Kind, &h.Summary, &h.Severity, &h.VulnClass, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SearchGlobalKnowledge exposes the DB-wide search on an exploration handle, so the
// agent ToolSet (which only holds an ExplorationStore) can offer it as a tool
// without importing *DB.
func (s *ExplorationStore) SearchGlobalKnowledge(q string, limit int) ([]GlobalKnowledgeHit, error) {
	return s.db.SearchGlobalKnowledge(q, limit)
}
