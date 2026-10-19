package agent

import (
	"regexp"
	"strings"
)

// SkillRelevanceNudgeEnabled reports whether graph_overview's
// "unused_relevant_skills" signal is active. Assigned once at startup
// (server/assembly.go's wireTools); nil until then, which
// skillRelevanceNudgeEnabled treats as off.
var SkillRelevanceNudgeEnabled func() bool

func skillRelevanceNudgeEnabled() bool {
	return SkillRelevanceNudgeEnabled != nil && SkillRelevanceNudgeEnabled()
}

// SkillCatalogDir returns the root directory of all skill subdirectories
// (the same path server.New passes to wireAgentAugment). Assigned once at
// startup; nil until then, which skillCatalogDir treats as "" (no catalog).
var SkillCatalogDir func() string

func skillCatalogDir() string {
	if SkillCatalogDir == nil {
		return ""
	}
	return SkillCatalogDir()
}

var skillTokenRe = regexp.MustCompile(`[a-z0-9]+`)

// addTokens lowercases text, extracts alphanumeric runs, and adds every run
// of length >= 4 to set -- a simple stopword-style filter with no maintained
// word list ("the"/"and"/"for"/"has" all fall below the length floor).
func addTokens(set map[string]bool, text string) {
	for _, tok := range skillTokenRe.FindAllString(strings.ToLower(text), -1) {
		if len(tok) >= 4 {
			set[tok] = true
		}
	}
}
