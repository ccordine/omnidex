package webresearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/specialistworkflow"
	"github.com/gryph/omnidex/internal/websearch"
)

func (machine *Machine) gatherRelevantEvidence(ctx context.Context, result *Result) error {
	if machine == nil || result == nil {
		return fmt.Errorf("%w: evidence gathering authority is unavailable", ErrInvalidConfiguration)
	}
	attempts, err := specialistworkflow.NewAttemptBudget(uint16(machine.config.MaxSearchTerms + 3))
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
		candidates, err = machine.resolveSearchTerms(ctx, attempts, result)
		if err != nil {
			return err
		}
		documents, err = machine.fetch(ctx, attempts, candidates, result)
		if err != nil {
			return fmt.Errorf("%w after bounded search-term resolution: %w", ErrEvidenceUnavailable, err)
		}
		result.Steps = append(result.Steps, StepDocumentsFetched)
	}
	for {
		evidence := evidenceFromDocuments(documents)
		result.Evidence = cloneEvidence(evidence)
		projected, relevant, selectErr := machine.selectAndProject(ctx, evidence, result)
		if selectErr != nil {
			return selectErr
		}
		if relevant {
			result.Evidence, err = applyProjectionTruncation(result.Evidence, projected)
			if err != nil {
				return err
			}
			result.Projected = cloneProjection(projected)
			result.Steps = append(result.Steps, StepEvidenceProjected)
			return nil
		}
		if result.SearchTermsCalls > 0 {
			return fmt.Errorf("%w after bounded relevance and search-term resolution", ErrEvidenceUnavailable)
		}
		candidates, err = machine.resolveSearchTerms(ctx, attempts, result)
		if err != nil {
			return err
		}
		documents, err = machine.fetch(ctx, attempts, candidates, result)
		if err != nil {
			return fmt.Errorf("%w after bounded relevance search-term resolution: %w", ErrEvidenceUnavailable, err)
		}
		result.Steps = append(result.Steps, StepDocumentsFetched)
	}
}
