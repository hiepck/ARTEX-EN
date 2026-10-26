package agent

import (
	"fmt"
	"strings"

	"github.com/Autumn-27/artex/db"
)

// Live-direction dedup at intent admission.
//
// Why a hard guard and not just a prompt instruction: the planner prompt already
// says "do not duplicate an existing intent", but that is advisory — in production
// two intents with the same endpoint+technique were dispatched in the same round,
// which puts two workers on one direction and wastes an LLM slot (the scarce
// resource in a long hunt). This guard is deterministic and cheap.
//
// Deliberately narrow, because a false block costs hunt coverage:
//   - only OPEN / RUNNING / PAUSED intents are compared. Finished intents
//     (done/exhausted/blocked/stopped) are NOT considered — re-running a direction
//     after new evidence ("every prior negative is suspect") is a legitimate and
//     valuable planner move, and blocking it would reduce exploitation depth.
//   - similarity is a token-set Jaccard over distinctive words (len>=4, stopwords
//     removed). Near-identical summaries share hostnames/endpoints/technique words
//     and score high; "test X for IDOR" vs "test X for SQLi" share the target but
//     diverge on the technique, so they stay below the threshold.
//   - both a ratio and an absolute shared-token floor must be met, so two short
//     summaries that happen to share one word never block each other.
const (
	intentDedupThreshold = 0.60
	intentDedupMinShared = 6
)

// intentStopwords removes the high-frequency words that would otherwise inflate
// the overlap between two unrelated directions (both summaries are English-ish
// prose). Kept intentionally small — only words with no discriminating power.
var intentStopwords = map[string]bool{
	"this": true, "that": true, "these": true, "those": true, "there": true, "their": true,
	"with": true, "from": true, "into": true, "over": true, "than": true, "then": true,
	"have": true, "been": true, "they": true, "will": true, "would": true, "should": true,
	"when": true, "what": true, "which": true, "where": true, "while": true, "after": true,
	"before": true, "about": true, "every": true, "never": true, "under": true, "also": true,
	"only": true, "just": true, "more": true, "most": true, "some": true, "such": true,
	"very": true, "each": true, "other": true, "because": true, "same": true, "already": true,
	"still": true, "does": true, "doing": true, "make": true, "made": true, "using": true,
	"used": true, "work": true, "works": true, "intent": true, "intents": true,
	"fact": true, "facts": true, "task": true, "tasks": true,
}

// intentTokens lowercases and tokenizes a summary into its distinctive word set.
// It reuses addTokens (lowercase, tokens of length >= 4) then drops stopwords.
func intentTokens(text string) map[string]bool {
	set := map[string]bool{}
	addTokens(set, text)
	for w := range set {
		if intentStopwords[w] {
			delete(set, w)
		}
	}
	return set
}

// intentSimilarity returns the Jaccard similarity of two token sets plus the
// number of shared tokens (the absolute floor that keeps short summaries honest).
func intentSimilarity(a, b map[string]bool) (ratio float64, shared int) {
	if len(a) == 0 || len(b) == 0 {
		return 0, 0
	}
	for tok := range a {
		if b[tok] {
			shared++
		}
	}
	union := len(a) + len(b) - shared
	if union == 0 {
		return 0, 0
	}
	return float64(shared) / float64(union), shared
}

// nodeSummary is defined in compaction.go (payload summary, falling back to text)
// and reused here so the two readers cannot drift apart.

// liveIntentDuplicate returns the live (open/running/paused) intent whose summary
// is most similar to the candidate, when that similarity is high enough that
// dispatching it would put two workers on the same direction. Returns nil when no
// live intent is close enough, or when the candidate is too short to judge.
func liveIntentDuplicate(intents []*db.Node, summary string) (*db.Node, float64) {
	cand := intentTokens(summary)
	// Too little signal to judge safely: a three-word summary cannot establish that
	// two directions are the same, so never block on it.
	if len(cand) < intentDedupMinShared {
		return nil, 0
	}
	var (
		best      *db.Node
		bestRatio float64
	)
	for _, n := range intents {
		if n == nil || n.Kind != db.KindIntent {
			continue
		}
		switch n.State {
		case "open", "running", "paused":
		default:
			continue // finished directions may legitimately be re-run
		}
		ratio, shared := intentSimilarity(cand, intentTokens(nodeSummary(n)))
		if shared < intentDedupMinShared || ratio < intentDedupThreshold {
			continue
		}
		if best == nil || ratio > bestRatio {
			best, bestRatio = n, ratio
		}
	}
	return best, bestRatio
}

// intentDuplicateError renders the admission rejection. It is written for the
// planner model to act on: it names the live intent, its state, and the concrete
// options (different angle / steer it / wait), rather than a bare "duplicate".
func intentDuplicateError(dup *db.Node, ratio float64) error {
	summary := strings.TrimSpace(nodeSummary(dup))
	if len(summary) > 220 {
		summary = summary[:220] + "…"
	}
	return fmt.Errorf("intent not created: the %s intent %d already covers this direction (%.0f%% token overlap): %q. "+
		"Do not dispatch a second worker on it. Either (a) make the new direction genuinely different (state what is uncovered that this intent does not test), "+
		"(b) adjust the live one with steer_work if the angle is close but should change, or (c) wait for it to finish and judge from its facts/findings.",
		dup.State, dup.ID, ratio*100, summary)
}
