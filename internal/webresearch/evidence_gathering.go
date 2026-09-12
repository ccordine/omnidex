package webresearch

import (
	"context"
	"fmt"
)

func (machine *evidenceMachine) gatherRelevantEvidence(ctx context.Context, result *evidenceRun) error {
	if machine == nil || result == nil {
		return fmt.Errorf("%w: evidence gathering authority is unavailable", ErrInvalidConfiguration)
	}
	candidates, err := machine.initialAcquisition(ctx, result)
	if err != nil {
		return err
	}
	documents, err := machine.fetch(ctx, candidates, result)
	if err != nil {
		return err
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
	result.Projected = cloneProjection(projected)
	return nil
}
