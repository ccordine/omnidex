package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/specialists"
	"github.com/gryph/omnidex/internal/station"
)

const codingSkillCandidateLimit = 5

func (s *directCodingSession) bindRequirementSkill(
	requirement assemblyline.Requirement,
	input codingSkillNeed,
	index int,
) (directCodingSkillBinding, bool, error) {
	exact, exists, err := s.runtime.svc.repo.ActiveLearnedSkill(
		s.runtime.ctx, learnedCodingSkillID(input),
	)
	if err != nil {
		return directCodingSkillBinding{}, false, err
	}
	if exists {
		if exact.Spec.Purpose != codingSkillPurpose(input) {
			return directCodingSkillBinding{}, false, fmt.Errorf(
				"exact learned skill %s purpose does not match its code-owned identity", exact.Spec.ID,
			)
		}
		return s.activeSkillBinding(requirement.ID, exact, "exact"), true, nil
	}
	hasActiveSkills, err := s.runtime.svc.repo.HasActiveWorkerSkills(s.runtime.ctx)
	if err != nil {
		return directCodingSkillBinding{}, false, err
	}
	if !hasActiveSkills {
		return directCodingSkillBinding{}, false, nil
	}
	if strings.TrimSpace(s.runtime.svc.embeddingProvider) == "" ||
		strings.TrimSpace(s.runtime.svc.embeddingModel) == "" {
		return directCodingSkillBinding{}, false, fmt.Errorf(
			"coding skill retrieval requires embedding provider and model authority",
		)
	}
	hasCandidates, err := s.runtime.svc.repo.HasActiveWorkerSkillEmbeddings(
		s.runtime.ctx, s.runtime.svc.embeddingProvider, s.runtime.svc.embeddingModel,
	)
	if err != nil {
		return directCodingSkillBinding{}, false, err
	}
	if !hasCandidates {
		return directCodingSkillBinding{}, false, nil
	}
	if s.runtime.svc.embeddings == nil {
		return directCodingSkillBinding{}, false, fmt.Errorf("coding skill retrieval requires the configured embedding provider")
	}
	embedding, err := s.runtime.svc.embeddings.Embedding(s.runtime.ctx, codingSkillRetrievalText(input))
	if err != nil {
		return directCodingSkillBinding{}, false, fmt.Errorf("embed local need for %s: %w", requirement.ID, err)
	}
	matches, err := s.runtime.svc.repo.FindActiveWorkerSkillMatches(
		s.runtime.ctx,
		s.runtime.svc.embeddingProvider,
		s.runtime.svc.embeddingModel,
		embedding,
		codingSkillCandidateLimit,
	)
	if err != nil {
		return directCodingSkillBinding{}, false, err
	}
	selected, err := s.selectActiveCodingSkill(requirement, input, matches, index)
	if err != nil {
		return directCodingSkillBinding{}, false, err
	}
	if selected != nil {
		return s.activeSkillBinding(requirement.ID, *selected, "semantic"), true, nil
	}
	return directCodingSkillBinding{}, false, nil
}

func (s *directCodingSession) selectActiveCodingSkill(
	requirement assemblyline.Requirement,
	procedureInput codingSkillNeed,
	matches []queue.WorkerSkillMatch,
	index int,
) (*specialists.SkillVersion, error) {
	if len(matches) == 0 {
		return nil, nil
	}
	modelName, err := stationModel(s.runtime.routing, station.CodingSkillSelection)
	if err != nil {
		return nil, err
	}
	modelName, err = requireDirectCodingModel(station.CodingSkillSelection, modelName)
	if err != nil {
		return nil, err
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
	decision, err := runDirectCodingSemanticLeafCall(
		directCodingWorkerRuntime(s), modelName, fmt.Sprintf("skill_selection_%03d", index+1),
		job, nil,
		func(raw string) (assemblyline.SkillSelectionDecision, error) {
			return assemblyline.DecodeSkillSelectionDecision(input, raw)
		},
		func(value assemblyline.SkillSelectionDecision) error { return value.ValidateFor(input) },
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
