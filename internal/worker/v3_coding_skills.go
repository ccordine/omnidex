package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialists"
)

type directCodingSkillBinding struct {
	RequirementID string
	Procedure     string
	Version       specialists.SkillVersion
}

type codingSkillNeed struct {
	LocalContext string
	Need         string
}

func (s *directCodingSession) bindRequirementSkills(
	localContext string,
	requirements []assemblyline.Requirement,
) (map[string]directCodingSkillBinding, error) {
	if s == nil || s.runtime == nil || s.runtime.svc == nil || s.runtime.svc.repo == nil {
		return nil, fmt.Errorf("coding skill retrieval requires the authoritative PostgreSQL registry")
	}
	bindings := make(map[string]directCodingSkillBinding, len(requirements))
	boundBySkillID := make(map[string]directCodingSkillBinding, len(requirements))
	unboundSkillIDs := make(map[string]struct{}, len(requirements))
	for index, requirement := range requirements {
		input := codingSkillNeed{
			LocalContext: localContext, Need: requirement.SourceQuote,
		}
		skillID := learnedCodingSkillID(input)
		if prior, exists := boundBySkillID[skillID]; exists {
			prior.RequirementID = requirement.ID
			bindings[requirement.ID] = prior
			continue
		}
		if _, exists := unboundSkillIDs[skillID]; exists {
			continue
		}
		binding, bound, err := s.bindRequirementSkill(requirement, input, index)
		if err != nil {
			return nil, err
		}
		if bound {
			bindings[requirement.ID] = binding
			boundBySkillID[skillID] = binding
		} else {
			unboundSkillIDs[skillID] = struct{}{}
		}
	}
	return bindings, nil
}

func learnedCodingSkillID(input codingSkillNeed) string {
	digest := sha256.Sum256([]byte(
		"typescript_react_view\x00" + input.LocalContext + "\x00" + input.Need,
	))
	return "learned_" + hex.EncodeToString(digest[:16])
}

func codingSkillPurpose(input codingSkillNeed) string {
	return "Local context: " + input.LocalContext + "\nLocal need: " + input.Need
}

func codingSkillRetrievalText(input codingSkillNeed) string {
	return input.LocalContext + "\n" + input.Need
}

func validateDirectCodingSkillBindings(
	requirements []assemblyline.Requirement,
	bindings map[string]directCodingSkillBinding,
) error {
	if len(bindings) > len(requirements) {
		return fmt.Errorf("coding skill bindings=%d exceed requirements=%d", len(bindings), len(requirements))
	}
	requirementIDs := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		requirementIDs[requirement.ID] = struct{}{}
		binding, exists := bindings[requirement.ID]
		if !exists {
			continue
		}
		if binding.RequirementID != requirement.ID || strings.TrimSpace(binding.Procedure) == "" {
			return fmt.Errorf("coding requirement %s has an invalid learned skill binding", requirement.ID)
		}
		if binding.Version.Status != specialists.SkillStatusActive {
			return fmt.Errorf("coding requirement %s learned skill is not active", requirement.ID)
		}
		if err := binding.Version.Validate(); err != nil {
			return fmt.Errorf(
				"coding requirement %s learned skill version is invalid: %w", requirement.ID, err,
			)
		}
		if binding.Procedure != binding.Version.Spec.Instructions {
			return fmt.Errorf(
				"coding requirement %s learned skill procedure does not match its immutable version",
				requirement.ID,
			)
		}
	}
	unknown := make([]string, 0)
	for requirementID := range bindings {
		if _, exists := requirementIDs[requirementID]; !exists {
			unknown = append(unknown, requirementID)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("learned skill binding targets unknown requirement %s", unknown[0])
	}
	return nil
}
