package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/specialists"
)

const codingSkillCandidateLimit = 5

func (s *directCodingSession) bindRequirementSkill(
	requirement assemblyline.Requirement,
	input assemblyline.SkillProcedureInput,
	index int,
	selectionModel string,
	procedureModel string,
) (directCodingSkillBinding, error) {
	exact, exists, err := s.runtime.svc.repo.ActiveLearnedSkill(
		s.runtime.ctx, learnedCodingSkillID(input),
	)
	if err != nil {
		return directCodingSkillBinding{}, err
	}
	if exists {
		if exact.Spec.Purpose != codingSkillPurpose(input) {
			return directCodingSkillBinding{}, fmt.Errorf(
				"exact learned skill %s purpose does not match its code-owned identity", exact.Spec.ID,
			)
		}
		return s.activeSkillBinding(requirement.ID, exact, "exact"), nil
	}
	embedding, err := s.runtime.svc.llm.Embedding(s.runtime.ctx, codingSkillRetrievalText(input))
	if err != nil {
		return directCodingSkillBinding{}, fmt.Errorf("embed local need for %s: %w", requirement.ID, err)
	}
	matches, err := s.runtime.svc.repo.FindActiveWorkerSkillMatches(
		s.runtime.ctx,
		s.runtime.svc.embeddingProvider,
		s.runtime.svc.embeddingModel,
		embedding,
		codingSkillCandidateLimit,
	)
	if err != nil {
		return directCodingSkillBinding{}, err
	}
	selected, err := s.selectActiveCodingSkill(requirement, input, matches, selectionModel, index)
	if err != nil {
		return directCodingSkillBinding{}, err
	}
	if selected != nil {
		return s.activeSkillBinding(requirement.ID, *selected, "semantic"), nil
	}
	return s.createCodingSkillCandidate(requirement, input, embedding, procedureModel, index)
}

func (s *directCodingSession) selectActiveCodingSkill(
	requirement assemblyline.Requirement,
	procedureInput assemblyline.SkillProcedureInput,
	matches []queue.WorkerSkillMatch,
	modelName string,
	index int,
) (*specialists.SkillVersion, error) {
	if len(matches) == 0 {
		return nil, nil
	}
	candidates := make([]assemblyline.SkillCandidateSummary, len(matches))
	versions := make(map[string]specialists.SkillVersion, len(matches))
	for candidateIndex, match := range matches {
		token := fmt.Sprintf("SKILL_%d", candidateIndex+1)
		candidates[candidateIndex] = assemblyline.SkillCandidateSummary{
			Token: token, Purpose: match.Version.Spec.Purpose,
		}
		versions[token] = match.Version
	}
	input := assemblyline.SkillSelectionInput{
		LocalContext: procedureInput.LocalContext,
		Need:         procedureInput.Need,
		Candidates:   candidates,
	}
	job, err := assemblyline.NewSkillSelectionJob(input)
	if err != nil {
		return nil, err
	}
	decision, err := runDirectCodingSemanticCall[assemblyline.SkillSelectionDecision](
		directCodingWorkerRuntime(s), modelName, fmt.Sprintf("skill_selection_%03d", index+1),
		job, nil, func(value assemblyline.SkillSelectionDecision) error { return value.ValidateFor(input) },
	)
	if err != nil {
		return nil, err
	}
	if decision.Selected == assemblyline.SkillSelectionNone {
		return nil, nil
	}
	version, exists := versions[decision.Selected]
	if !exists {
		return nil, fmt.Errorf("skill selection escaped its code-owned candidate map")
	}
	return &version, nil
}

func (s *directCodingSession) createCodingSkillCandidate(
	requirement assemblyline.Requirement,
	input assemblyline.SkillProcedureInput,
	embedding []float64,
	modelName string,
	index int,
) (directCodingSkillBinding, error) {
	job, err := assemblyline.NewSkillProcedureJob(input)
	if err != nil {
		return directCodingSkillBinding{}, err
	}
	decision, err := runDirectCodingSemanticCall[assemblyline.SkillProcedureDecision](
		directCodingWorkerRuntime(s), modelName, fmt.Sprintf("skill_procedure_%03d", index+1),
		job, nil, func(value assemblyline.SkillProcedureDecision) error { return value.ValidateFor(input) },
	)
	if err != nil {
		return directCodingSkillBinding{}, err
	}
	spec, err := newLearnedCodingSkillSpec(input, decision)
	if err != nil {
		return directCodingSkillBinding{}, err
	}
	stored, created, err := s.runtime.svc.repo.CreateLearnedSkillCandidateByStepAttempt(
		s.runtime.ctx, s.runtime.claim.Authority, spec,
	)
	if err != nil {
		return directCodingSkillBinding{}, fmt.Errorf("store learned skill for %s: %w", requirement.ID, err)
	}
	if err := s.runtime.svc.repo.StoreWorkerSkillEmbeddingByStepAttempt(
		s.runtime.ctx, s.runtime.claim.Authority, stored.Spec.ID, stored.Version,
		s.runtime.svc.embeddingProvider, s.runtime.svc.embeddingModel, embedding,
	); err != nil {
		return directCodingSkillBinding{}, err
	}
	if stored.Status == specialists.SkillStatusCandidate {
		if err := s.runtime.svc.repo.BeginWorkerSkillValidationByStepAttempt(
			s.runtime.ctx, s.runtime.claim.Authority, stored.Spec.ID, stored.Version, specialists.SkillCheck{
				Name: "contract", Status: specialists.SkillCheckPassed,
				Detail: "Bounded procedure and fixed coding schemas passed structural validation.",
			},
		); err != nil {
			return directCodingSkillBinding{}, err
		}
		stored.Status = specialists.SkillStatusValidating
	}
	pending := stored.Status == specialists.SkillStatusValidating
	if stored.Status != specialists.SkillStatusActive && !pending {
		return directCodingSkillBinding{}, fmt.Errorf(
			"learned skill %s returned unusable status %s", stored.Spec.ID, stored.Status,
		)
	}
	if pending {
		s.trackSkillCandidate(stored)
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_skill_bound", fmt.Sprintf(
		"requirement=%s skill=%s version=%d source=generated status=%s created=%t",
		requirement.ID, stored.Spec.ID, stored.Version, stored.Status, created,
	))
	return directCodingSkillBinding{
		RequirementID: requirement.ID, Procedure: stored.Spec.Instructions,
		Version: stored, Pending: pending,
	}, nil
}

func (s *directCodingSession) activeSkillBinding(
	requirementID string,
	version specialists.SkillVersion,
	source string,
) directCodingSkillBinding {
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_skill_bound", fmt.Sprintf(
		"requirement=%s skill=%s version=%d source=%s status=%s",
		requirementID, version.Spec.ID, version.Version, strings.TrimSpace(source), version.Status,
	))
	return directCodingSkillBinding{
		RequirementID: requirementID, Procedure: version.Spec.Instructions, Version: version,
	}
}
