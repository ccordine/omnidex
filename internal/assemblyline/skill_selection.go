package assemblyline

import (
	"fmt"
	"strings"
)

const (
	SkillSelectionSchemaV1 = "omnidex.skill-selection.v1"
	SkillSelectionNone     = "none"
	maxSkillCandidates     = 5
	maxSkillLocalContext   = 2000
	maxSkillPurposeBytes   = 1536
)

type SkillCandidateSummary struct {
	Token   string `json:"token"`
	Purpose string `json:"purpose"`
}

type SkillSelectionInput struct {
	LocalContext string                  `json:"local_context"`
	Need         string                  `json:"need"`
	Candidates   []SkillCandidateSummary `json:"candidates"`
}

type SkillSelectionDecision struct {
	Schema   string `json:"schema"`
	Selected string `json:"selected"`
}

func (input SkillSelectionInput) validate() error {
	if input.LocalContext == "" || input.LocalContext != strings.TrimSpace(input.LocalContext) ||
		len(input.LocalContext) > maxSkillLocalContext {
		return fmt.Errorf("skill selection requires one bounded trimmed local context")
	}
	if input.Need == "" || input.Need != strings.TrimSpace(input.Need) {
		return fmt.Errorf("skill selection requires one trimmed local need")
	}
	if len(input.Candidates) < 1 || len(input.Candidates) > maxSkillCandidates {
		return fmt.Errorf("skill selection requires between 1 and %d candidates", maxSkillCandidates)
	}
	seen := make(map[string]struct{}, len(input.Candidates))
	for index, candidate := range input.Candidates {
		wantToken := fmt.Sprintf("SKILL_%d", index+1)
		if candidate.Token != wantToken {
			return fmt.Errorf("skill candidate %d token must be %s", index, wantToken)
		}
		if candidate.Purpose == "" || candidate.Purpose != strings.TrimSpace(candidate.Purpose) ||
			len(candidate.Purpose) > maxSkillPurposeBytes {
			return fmt.Errorf("skill candidate %s has an invalid bounded purpose", candidate.Token)
		}
		if _, duplicate := seen[candidate.Purpose]; duplicate {
			return fmt.Errorf("skill candidate %s repeats another purpose", candidate.Token)
		}
		seen[candidate.Purpose] = struct{}{}
	}
	return nil
}

func (decision SkillSelectionDecision) ValidateFor(input SkillSelectionInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != SkillSelectionSchemaV1 {
		return fmt.Errorf("skill selection schema must be %q", SkillSelectionSchemaV1)
	}
	if decision.Selected == SkillSelectionNone {
		return nil
	}
	for _, candidate := range input.Candidates {
		if decision.Selected == candidate.Token {
			return nil
		}
	}
	return fmt.Errorf("selected skill token %q is outside the code-owned candidate set", decision.Selected)
}

func BuildSkillSelectionPrompt(input SkillSelectionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	lines := []string{
		"Select one existing skill only when its stated purpose directly covers the local need. Otherwise select none.",
		"Do not infer implementation details, combine skills, rewrite the need, or choose by shared words alone.",
		"LOCAL_CONTEXT:\n" + input.LocalContext,
		"LOCAL_NEED:\n" + input.Need,
		"CANDIDATE_PURPOSES:",
	}
	for _, candidate := range input.Candidates {
		lines = append(lines, candidate.Token+": "+candidate.Purpose)
	}
	lines = append(lines, "Return exactly one candidate token or none.")
	return strings.Join(lines, "\n"), nil
}

func SkillSelectionResponseSchema(input SkillSelectionInput) map[string]any {
	values := []string{SkillSelectionNone}
	for _, candidate := range input.Candidates {
		values = append(values, candidate.Token)
	}
	return objectSchema(
		[]string{"schema", "selected"},
		map[string]any{
			"schema":   map[string]any{"type": "string", "const": SkillSelectionSchemaV1},
			"selected": map[string]any{"type": "string", "enum": values},
		},
	)
}
