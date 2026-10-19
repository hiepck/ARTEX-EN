package agent

import (
	"errors"
	"testing"

	"github.com/Autumn-27/norma/skill"
)

func TestMatchUnusedSkillsDegradesToEmptyOnInvokedSkillsError(t *testing.T) {
	goalTokens := map[string]bool{"coupon": true, "discount": true}
	skills := []skill.Skill{{Dir: "/catalog/hunt-business-logic", Description: "Covers coupon and discount abuse."}}
	sug := matchUnusedSkills(goalTokens, skills, nil, errors.New("boom"), 2, 5)
	if len(sug) != 0 {
		t.Fatalf("matchUnusedSkills with invokedErr != nil = %#v, want empty (never show everything as unused on an unverifiable error)", sug)
	}
}

func TestMatchUnusedSkillsMatchesOnSuccess(t *testing.T) {
	goalTokens := map[string]bool{"coupon": true, "discount": true}
	skills := []skill.Skill{{Dir: "/catalog/hunt-business-logic", Description: "Covers coupon and discount abuse."}}
	sug := matchUnusedSkills(goalTokens, skills, map[string]bool{}, nil, 2, 5)
	if len(sug) != 1 || sug[0]["skill"] != "hunt-business-logic" {
		t.Fatalf("matchUnusedSkills on success = %#v, want [hunt-business-logic]", sug)
	}
}
