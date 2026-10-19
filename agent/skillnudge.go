package agent

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Autumn-27/norma/skill"
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

// matchUnusedSkills scores every skill's name+description against goalTokens
// and returns the top matches not already in invoked, as {skill, matched_terms}
// entries. If invokedErr is non-nil, the invoked set could not be verified --
// this degrades to an empty result (never shows everything as unused just
// because the ledger lookup itself failed), the same degrade shape as a
// skill.LoadDir failure in the caller.
func matchUnusedSkills(goalTokens map[string]bool, skills []skill.Skill, invoked map[string]bool, invokedErr error, minMatchTokens, maxSuggestions int) []map[string]any {
	sug := make([]map[string]any, 0)
	if invokedErr != nil {
		return sug
	}
	type scoredSkill struct {
		name    string
		score   int
		matched []string
	}
	var candidates []scoredSkill
	for _, sk := range skills {
		name := filepath.Base(sk.Dir)
		if invoked[name] {
			continue
		}
		skillTokens := map[string]bool{}
		addTokens(skillTokens, strings.ReplaceAll(name, "-", " "))
		addTokens(skillTokens, sk.Description)
		var matched []string
		for tok := range goalTokens {
			if skillTokens[tok] {
				matched = append(matched, tok)
			}
		}
		if len(matched) >= minMatchTokens {
			sort.Strings(matched)
			candidates = append(candidates, scoredSkill{name, len(matched), matched})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > maxSuggestions {
		candidates = candidates[:maxSuggestions]
	}
	for _, c := range candidates {
		sug = append(sug, map[string]any{"skill": c.name, "matched_terms": c.matched})
	}
	return sug
}
