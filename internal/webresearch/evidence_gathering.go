package webresearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/specialistworkflow"
	"github.com/gryph/omnidex/internal/websearch"
)

func (machine *evidenceMachine) gatherRelevantEvidence(ctx context.Context, result *evidenceRun) error {
	if machine == nil || result == nil {
		return fmt.Errorf("%w: evidence gathering authority is unavailable", ErrInvalidConfiguration)
	}
	attempts, err := specialistworkflow.NewAttemptBudget(2)
	if err != nil {
		return fmt.Errorf("%w: acquisition attempt budget: %w", ErrInvalidConfiguration, err)
	}
	candidates, initialUsable, err := machine.initialAcquisition(ctx, attempts, result)
	if err != nil {
		return err
	}
	var documents []websearch.Document
	if initialUsable {
		documents, err = machine.fetch(ctx, attempts, candidates, result)
		if err == nil {
			result.Steps = append(result.Steps, StepDocumentsFetched)
		} else if !errors.Is(err, websearch.ErrNoDocuments) ||
			!isRecoverableEmptyFetch(result.Fetches[len(result.Fetches)-1]) {
			return err
		}
	}
	if len(documents) == 0 {
		return fmt.Errorf(
			"%w: exact initial query %q produced no fetchable documents",
			ErrEvidenceUnavailable, machine.objective.InitialQuery,
		)
	}
	evidence := evidenceFromDocuments(documents)
	result.Evidence = cloneEvidence(evidence)
	projected, relevant, err := machine.selectAndProject(ctx, evidence, result)
	if err != nil {
		return err
	}
	if !relevant {
		return fmt.Errorf(
			"%w: exact initial query %q produced no relevant candidates",
			ErrEvidenceUnavailable, machine.objective.InitialQuery,
		)
	}
	result.Evidence, err = applyProjectionTruncation(result.Evidence, projected)
	if err != nil {
		return err
	}
	result.Projected = cloneProjection(projected)
	result.Steps = append(result.Steps, StepEvidenceProjected)
	return nil
}
