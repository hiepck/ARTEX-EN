package db

// Exploration node kinds (exploration_nodes.kind).
const (
	KindBegin   = "begin"   // DEPRECATED: legacy task root; new tasks seed an origin fact (KindFact + StateOrigin) instead
	KindGoal    = "goal"    // a task objective
	KindIntent  = "intent"  // an exploration direction (planner-generated)
	KindFact    = "fact"    // a worker's exploration result/conclusion (incl. negative results), tied to its intent
	KindFinding = "finding" // a confirmed vulnerability (report_finding), distinct from a fact
	KindHint    = "hint"
	KindDigest  = "digest" // a compressed fold of cold intents/facts (cold-digest-spec §1); lossless — members kept, restorable by id
)

// Digest node states (kind='digest'). A digest is 'active' while it renders in
// graph_overview; major compaction retires a merged-away segment to 'superseded'
// (its covers edges repointed to the new digest) — cold-digest-spec §5.1.
const (
	StateDigestActive     = "active"
	StateDigestSuperseded = "superseded"
)

// StateOrigin marks the task root fact (KindFact) seeded at task creation — the
// exploration graph's origin. Every intent traces back to it, so "an intent must
// connect to a fact" holds uniformly from the very first intent. Worker-produced
// facts use state 'confirmed', so this never collides.
const StateOrigin = "origin"

// StateIntentDeleted marks an intent the user soft-deleted: it drops out of
// the frontier and graph_overview like other terminal states, but keeps its node
// and full lineage. The delete reason lives in exploration_nodes.delete_reason.
const StateIntentDeleted = "deleted"

// Exploration edge relations (exploration_edges.rel).
const (
	RelSpawns      = "spawns"
	RelDerivedFrom = "derived_from"
	RelYields      = "yields"
	RelProves      = "proves"
	RelCovers      = "covers"     // digest --covers--> member (cold-digest-spec §1); source of truth for "which digest folds node X"
	RelSupersedes  = "supersedes" // fact --supersedes--> fact: a later fact overturns an earlier one's conclusion (this exploration only)
	// RelSharesRootCause: finding --shares_root_cause--> finding: a worker's judgment
	// that two confirmed findings trace back to the same underlying flaw (this exploration only)
	RelSharesRootCause = "shares_root_cause"
)

// Asset-edge relations (asset_edges.rel) -- a different table and vocabulary
// from the exploration_edges Rel* constants above; do not conflate the two.
const (
	AssetRelCanReach = "can_reach"
)
