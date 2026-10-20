package server

import (
	"context"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/db"
)

// requireEmptyLLMProfiles skips the calling test rather than ever deleting
// existing llm_profiles rows. That table holds real, often irreplaceable
// provider configuration (including API keys this process never sees in
// plaintext once stored) -- a test that needs to observe the "nothing
// configured yet" state must only run against a database that already
// starts with an empty profiles table (a fresh CI/test database), never
// force that state by wiping real rows on a shared or production instance.
// (An earlier version of this file's own TestChatUnavailableReasonDistinguishesStates
// did exactly that wipe, assuming test isolation; run once against a shared
// production Postgres, it deleted every real LLM profile with no backup to
// recover from -- see the incident this guard exists to prevent.)
func requireEmptyLLMProfiles(t *testing.T, pg *db.DB) {
	t.Helper()
	existing, err := pg.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(existing) > 0 {
		t.Skipf("skipping: %d pre-existing llm_profiles row(s) found -- this test requires starting from an empty profiles table and must never delete real rows to force that state (run against a fresh/isolated test database instead)", len(existing))
	}
}

// TestChatUnavailableReasonDistinguishesStates pins the operator-facing message
// to the actual configuration state: nothing configured vs. configured-but-not-
// activated must read differently so the user knows what to do.
func TestChatUnavailableReasonDistinguishesStates(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	requireEmptyLLMProfiles(t, m.pg)
	td := t.TempDir()
	s := New(context.Background(), m, td, td, td)

	if reason := s.chatUnavailableReason(); !strings.Contains(reason, "No LLM is configured yet") {
		t.Fatalf("no-profile reason=%q, want \"No LLM is configured yet\"", reason)
	}

	id, err := m.pg.SaveProfile(&db.LLMProfile{Name: "p1", Format: "anthropic", Model: "claude-x", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = m.pg.Exec(`UPDATE llm_profiles SET is_default=false WHERE id=$1`, id)
		_ = m.pg.DeleteProfile(id)
	})

	// Profile exists but is not activated.
	if reason := s.chatUnavailableReason(); !strings.Contains(reason, "No LLM configuration is active") {
		t.Fatalf("inactive reason=%q, want \"No LLM configuration is active\"", reason)
	}

	// Once activated, the reason no longer claims a missing/inactive config.
	if err := m.pg.SetActiveProfile(id); err != nil {
		t.Fatal(err)
	}
	if reason := s.chatUnavailableReason(); strings.Contains(reason, "No LLM is configured yet") || strings.Contains(reason, "No LLM configuration is active") {
		t.Fatalf("active reason=%q should not report missing/inactive", reason)
	}
}

// TestResolveChatAgentHonoursConversationProfile is the regression guard for the
// reported bug: a conversation that picked a valid profile must resolve a chat
// agent even when NO global profile is active, so the send precheck stops
// rejecting it with "the LLM is not configured".
func TestResolveChatAgentHonoursConversationProfile(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer m.Close()
	td := t.TempDir()
	s := New(context.Background(), m, td, td, td)

	// Force the global fallback to nil so a non-nil result can ONLY come from the
	// conversation's own profile — this is exactly the situation the user hit
	// (no usable global config) and isolates the fix from the test host's env.
	s.cfgMu.Lock()
	s.chatAgent = nil
	s.cfgMu.Unlock()

	id, err := m.pg.SaveProfile(&db.LLMProfile{Name: "conv-pick", Format: "anthropic", Model: "claude-x", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.pg.DeleteProfile(id) })

	// A conversation that pinned this profile resolves a chat agent despite the
	// nil global fallback; without the fix the send precheck rejected it outright.
	conv := &db.Conversation{AgentKey: "mainagent", LLMProfileID: &id}
	if s.resolveChatAgent(conv) == nil {
		t.Fatalf("resolveChatAgent(conversation with valid profile) = nil; global fallback wrongly gates the pick")
	}

	// With neither a pick nor a global fallback there is genuinely nothing to run.
	if s.resolveChatAgent(&db.Conversation{AgentKey: "mainagent"}) != nil {
		t.Fatalf("resolveChatAgent with neither pick nor global config should be nil")
	}
}
