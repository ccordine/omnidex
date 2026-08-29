package webresearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/specialistworkflow"
	"github.com/gryph/omnidex/internal/websearch"
)

func newFetchContract(
	registration specialistworkflow.Registration,
	acquisition Acquisition,
	maxFetchCandidates int,
) (specialistworkflow.Contract[fetchState, fetchAttemptConfig, fetchObservation, acquisitionFailure], error) {
	return specialistworkflow.NewContract(
		registration,
		func(state fetchState) (fetchAttemptConfig, error) {
			return fetchAttemptConfig{candidates: state.candidates}, nil
		},
		func(config fetchAttemptConfig) error {
			return validateFetchCandidates(config.candidates, maxFetchCandidates)
		},
		func(ctx context.Context, config fetchAttemptConfig) (fetchObservation, error) {
			if boundsErr := validateCandidateSliceBounds(config.candidates); boundsErr != nil {
				return fetchObservation{}, fmt.Errorf("fetch configuration bounds: %w", boundsErr)
			}
			ids := make([]websearch.CandidateID, len(config.candidates))
			for index, candidate := range config.candidates {
				ids[index] = candidate.ID
			}
			report, err := acquisition.Fetch(ctx, websearch.FetchRequest{
				Candidates:   cloneCandidates(config.candidates),
				CandidateIDs: append([]websearch.CandidateID{}, ids...),
			})
			if boundsErr := validateDocumentReportBounds(report); boundsErr != nil {
				return fetchObservation{}, fmt.Errorf("fetch report bounds: %w", boundsErr)
			}
			observation := fetchObservation{report: cloneDocumentReport(report)}
			switch {
			case err == nil:
				observation.outcome = fetchObservedDocuments
				return observation, nil
			case errors.Is(err, websearch.ErrNoDocuments):
				observation.outcome = fetchObservedEmpty
				return observation, nil
			default:
				return fetchObservation{}, err
			}
		},
		func(_ context.Context, config fetchAttemptConfig, observation fetchObservation) (bool, error) {
			var acquisitionErr error
			if observation.outcome == fetchObservedEmpty {
				acquisitionErr = websearch.ErrNoDocuments
			}
			if err := validateDocumentReport(observation.report, config.candidates, acquisitionErr); err != nil {
				return false, err
			}
			switch observation.outcome {
			case fetchObservedDocuments:
				return true, nil
			case fetchObservedEmpty:
				return false, nil
			default:
				return false, fmt.Errorf("unknown fetch outcome %q", observation.outcome)
			}
		},
		func(_ context.Context, _ fetchAttemptConfig, observation fetchObservation) (acquisitionFailure, error) {
			if observation.outcome != fetchObservedEmpty {
				return acquisitionFailure{}, fmt.Errorf("cannot reduce fetch outcome %q", observation.outcome)
			}
			return acquisitionFailure{kind: failureNoDocuments}, nil
		},
	)
}

func validateFetchCandidates(candidates []websearch.Candidate, limit int) error {
	if len(candidates) == 0 || len(candidates) > limit {
		return fmt.Errorf("candidate count %d exceeds 1..%d", len(candidates), limit)
	}
	seen := make(map[websearch.CandidateID]struct{}, len(candidates))
	for _, candidate := range candidates {
		if err := websearch.ValidateCandidate(candidate); err != nil {
			return err
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			return fmt.Errorf("duplicate candidate %q", candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
	}
	return nil
}
