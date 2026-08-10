package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/specialists"
)

type directCodingSkillBinding struct {
	RequirementID string
	Procedure     string
	Version       specialists.SkillVersion
	Pending       bool
}

func (s *directCodingSession) bindRequirementSkills(
	localContext string,
	requirements []assemblyline.Requirement,
) (map[string]directCodingSkillBinding, error) {
	if s == nil || s.runtime == nil || s.runtime.svc == nil || s.runtime.svc.repo == nil {
		return nil, fmt.Errorf("coding skill synthesis requires the authoritative PostgreSQL registry")
	}
	procedureModel := s.runtime.svc.v3SpecialistModel(
		s.runtime.claim.Job,
		s.runtime.routing,
		"coding_skill_procedure",
		specialist.RoleCodingSkillProcedureStation,
		s.runtime.routing.Glue,
	)
	procedureModel, err := requireDirectCodingModel(specialist.RoleCodingSkillProcedureStation, procedureModel)
	if err != nil {
		return nil, err
	}
	selectionModel := s.runtime.svc.v3SpecialistModel(
		s.runtime.claim.Job,
		s.runtime.routing,
		"coding_skill_selection",
		specialist.RoleCodingSkillSelectionStation,
		s.runtime.routing.Glue,
	)
	selectionModel, err = requireDirectCodingModel(specialist.RoleCodingSkillSelectionStation, selectionModel)
	if err != nil {
		return nil, err
	}
	bindings := make(map[string]directCodingSkillBinding, len(requirements))
	boundBySkillID := make(map[string]directCodingSkillBinding, len(requirements))
	for index, requirement := range requirements {
		input := assemblyline.SkillProcedureInput{
			LocalContext: localContext, Need: requirement.SourceQuote,
			Boundary: assemblyline.SkillBoundaryTypeScriptReactView,
		}
		skillID := learnedCodingSkillID(input)
		if prior, exists := boundBySkillID[skillID]; exists {
			prior.RequirementID = requirement.ID
			bindings[requirement.ID] = prior
			continue
		}
		binding, err := s.bindRequirementSkill(
			requirement, input, index, selectionModel, procedureModel,
		)
		if err != nil {
			return nil, err
		}
		bindings[requirement.ID] = binding
		boundBySkillID[skillID] = binding
	}
	return bindings, nil
}

func newLearnedCodingSkillSpec(
	input assemblyline.SkillProcedureInput,
	decision assemblyline.SkillProcedureDecision,
) (specialists.Spec, error) {
	if err := decision.ValidateFor(input); err != nil {
		return specialists.Spec{}, err
	}
	spec := specialists.Spec{
		ID: learnedCodingSkillID(input), Purpose: codingSkillPurpose(input), Instructions: decision.Procedure,
		PreferredModel: []string{"coding_fragment"}, ContextBudget: 3 * 1024,
		RetryPolicy: "bounded_local_correction",
	}
	return specialists.SpecWithSchemaDocuments(
		spec,
		json.RawMessage(`{"type":"object","required":["signature","behavior","capabilities"],"properties":{"signature":{"type":"string"},"behavior":{"type":"string"},"capabilities":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`),
		json.RawMessage(`{"type":"object","required":["declaration"],"properties":{"declaration":{"type":"string"}},"additionalProperties":false}`),
	)
}

func learnedCodingSkillID(input assemblyline.SkillProcedureInput) string {
	digest := sha256.Sum256([]byte(
		string(input.Boundary) + "\x00" + input.LocalContext + "\x00" + input.Need,
	))
	return "learned_" + hex.EncodeToString(digest[:16])
}

func codingSkillPurpose(input assemblyline.SkillProcedureInput) string {
	return "Local context: " + input.LocalContext + "\nLocal need: " + input.Need
}

func codingSkillRetrievalText(input assemblyline.SkillProcedureInput) string {
	return input.LocalContext + "\n" + input.Need
}

func validateDirectCodingSkillBindings(
	requirements []assemblyline.Requirement,
	bindings map[string]directCodingSkillBinding,
) error {
	if len(bindings) != len(requirements) {
		return fmt.Errorf("coding skill bindings=%d do not cover requirements=%d", len(bindings), len(requirements))
	}
	for _, requirement := range requirements {
		binding, exists := bindings[requirement.ID]
		if !exists {
			return fmt.Errorf("coding requirement %s has no learned skill binding", requirement.ID)
		}
		if binding.RequirementID != requirement.ID || strings.TrimSpace(binding.Procedure) == "" {
			return fmt.Errorf("coding requirement %s has an invalid learned skill binding", requirement.ID)
		}
	}
	return nil
}

func (s *directCodingSession) recordPendingSkillCheck(name, detail string) error {
	for _, version := range s.skillCandidates {
		if err := s.runtime.svc.repo.RecordWorkerSkillCheckByStepAttempt(
			s.runtime.ctx, s.runtime.claim.Authority, version.Spec.ID, version.Version, specialists.SkillCheck{
				Name: name, Status: specialists.SkillCheckPassed, Detail: detail,
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *directCodingSession) activatePendingSkills() error {
	if len(s.skillCandidates) == 0 {
		return nil
	}
	if err := s.recordPendingSkillCheck(
		"workspace_verification", "Authoritative workspace tests, type checks, and production build passed.",
	); err != nil {
		return err
	}
	for len(s.skillCandidates) > 0 {
		version := s.skillCandidates[0]
		if err := s.runtime.svc.repo.ActivateWorkerSkillByStepAttempt(
			s.runtime.ctx, s.runtime.claim.Authority, version.Spec.ID, version.Version,
		); err != nil {
			return err
		}
		s.skillCandidates = s.skillCandidates[1:]
		s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_skill_activated", fmt.Sprintf(
			"skill=%s version=%d", version.Spec.ID, version.Version,
		))
	}
	return s.runtime.svc.refreshSkillRegistry(s.runtime.ctx)
}

func (s *directCodingSession) trackSkillCandidate(version specialists.SkillVersion) {
	for _, existing := range s.skillCandidates {
		if existing.Spec.ID == version.Spec.ID && existing.Version == version.Version {
			return
		}
	}
	s.skillCandidates = append(s.skillCandidates, version)
}

func (s *directCodingSession) rejectPendingSkills(cause error) error {
	if len(s.skillCandidates) == 0 {
		return nil
	}
	detail := "Validation stopped before the learned procedure produced a verified workspace."
	if cause != nil {
		detail += " " + trimForBudget(cause.Error(), 1200)
	}
	var failures []string
	for len(s.skillCandidates) > 0 {
		version := s.skillCandidates[0]
		err := s.runtime.svc.repo.RejectWorkerSkillByStepAttempt(
			s.runtime.ctx, s.runtime.claim.Authority, version.Spec.ID, version.Version, specialists.SkillCheck{
				Name: "workflow_failure", Status: specialists.SkillCheckFailed, Detail: detail,
			},
		)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s v%d: %v", version.Spec.ID, version.Version, err))
		} else {
			s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_skill_rejected", fmt.Sprintf(
				"skill=%s version=%d", version.Spec.ID, version.Version,
			))
		}
		s.skillCandidates = s.skillCandidates[1:]
	}
	if len(failures) > 0 {
		return fmt.Errorf("reject pending coding skills: %s", strings.Join(failures, "; "))
	}
	return nil
}
