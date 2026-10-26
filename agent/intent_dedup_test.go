package agent

import (
	"encoding/json"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func mkIntent(id int64, state, summary string) *db.Node {
	payload, _ := json.Marshal(map[string]any{"summary": summary})
	return &db.Node{ID: id, Kind: db.KindIntent, State: state, Payload: payload}
}

// The guard must block a second worker on a live direction, because that is the
// observed production waste (two intents dispatched on the same endpoint+technique
// in one planner round). It must NOT block a direction that only shares the target
// but tests a different technique, and it must NOT block re-running a finished
// direction (that is how a hunt retries after new evidence).
func TestLiveIntentDuplicate(t *testing.T) {
	t.Parallel()
	live := []*db.Node{
		mkIntent(101, "open", "Default-credential sweep on the apigiare.vn New-API Go backend (origin 45.32.109.193): POST /api/user/login with root:123456, root:root, admin:123456, newapi:123456, spaced >=5s apart"),
		mkIntent(102, "running", "Test the api.apimaster.ai /v1/chat/completions token minting path for quota bypass via unlimited_quota tokens"),
		mkIntent(103, "done", "Test the apimaster.ai /api/fingerprint endpoint for SSRF via base_url with loopback and link-local targets"),
		mkIntent(104, "exhausted", "Enumerate the marketplace endpoints on api.apimaster.ai for unauthenticated pricing data exposure"),
	}

	tests := []struct {
		name    string
		summary string
		wantID  int64 // 0 = must not be blocked
	}{
		{
			name:    "near-identical live direction is blocked",
			summary: "Default-credential sweep on the apigiare.vn New-API Go backend (origin 45.32.109.193): POST /api/user/login with root:123456, root:root, admin:123456, newapi:123456",
			wantID:  101,
		},
		{
			name:    "running intent is treated the same as open",
			summary: "Test the api.apimaster.ai /v1/chat/completions token minting path for quota bypass via unlimited_quota tokens",
			wantID:  102,
		},
		{
			name:    "same target, different technique is allowed",
			summary: "Test the apimaster.ai /api/fingerprint endpoint for mass assignment of the claimed_model field to bypass pricing",
			wantID:  0,
		},
		{
			name:    "finished direction may be re-run",
			summary: "Test the apimaster.ai /api/fingerprint endpoint for SSRF via base_url with loopback and link-local targets",
			wantID:  0, // the only exact match (103) is done; 102 differs enough? see note below
		},
		{
			name:    "short summary is never blocked",
			summary: "Check SSRF",
			wantID:  0,
		},
		{
			name:    "unrelated direction is allowed",
			summary: "Probe the Telegram webhook secret in the apimaster-ai production environment for callback forgery",
			wantID:  0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dup, ratio := liveIntentDuplicate(live, tt.summary)
			got := int64(0)
			if dup != nil {
				got = dup.ID
			}
			if got != tt.wantID {
				t.Fatalf("liveIntentDuplicate(%q) = intent %d (%.2f), want %d", tt.summary, got, ratio, tt.wantID)
			}
		})
	}
}

// A finished-only match must stay allowed even when the wording is identical:
// this is the "re-run after new evidence" path that the dedup guard must not take
// away from the planner.
func TestLiveIntentDuplicate_FinishedOnly(t *testing.T) {
	t.Parallel()
	const summary = "Test the apimaster.ai /api/fingerprint endpoint for SSRF via base_url with loopback and link-local targets"
	finished := []*db.Node{
		mkIntent(201, "done", summary),
		mkIntent(202, "exhausted", summary),
		mkIntent(203, "blocked", summary),
		mkIntent(204, "stopped", summary),
	}
	if dup, ratio := liveIntentDuplicate(finished, summary); dup != nil {
		t.Fatalf("a finished-only match must not block a re-run, got intent %d (%.2f)", dup.ID, ratio)
	}
}

// Two intents that merely share a target hostname but nothing else must not
// collide — the shared-token floor and the ratio threshold both have to hold.
func TestIntentSimilarity_SharedTargetOnly(t *testing.T) {
	t.Parallel()
	a := intentTokens("Test api.example.com login for SQL injection in the email parameter")
	b := intentTokens("Test api.example.com upload endpoint for path traversal in the filename")
	ratio, shared := intentSimilarity(a, b)
	if ratio >= intentDedupThreshold && shared >= intentDedupMinShared {
		t.Fatalf("shared-target-only summaries must not reach the dedup threshold: ratio=%.2f shared=%d", ratio, shared)
	}
}
