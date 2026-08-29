package webresearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/specialistworkflow"
	"github.com/gryph/omnidex/internal/websearch"
)

func (machine *Machine) initialAcquisition(
	ctx context.Context,
	attempts *specialistworkflow.AttemptBudget,
	result *Result,
) ([]websearch.Candidate, bool, error) {
	if machine.objective.InitialQuery == "" {
		return nil, false, fmt.Errorf("%w: exact initial query is unavailable", ErrInvalidObjective)
	}
	report, err := machine.discover(ctx, attempts, machine.objective.InitialQuery)
	recordDiscoveryAttempt(result, attempts)
	result.Discovery = append(result.Discovery, cloneCandidateReport(report))
	result.Steps = append(result.Steps, StepInitialDiscovery)
	if err == nil {
		candidates, reduceErr := reduceCandidates(report.Candidates, machine.config.MaxFetchCandidates)
		if reduceErr != nil {
			return nil, false, reduceErr
		}
		return candidates, true, nil
	}
	if !errors.Is(err, websearch.ErrNoCandidates) {
		return nil, false, fmt.Errorf("initial discovery: %w", err)
	}
	if everyProviderFailed(report) {
		return nil, false, fmt.Errorf("%w: every provider failed during initial discovery", ErrEvidenceUnavailable)
	}
	return nil, false, nil
}

func (machine *Machine) discover(
	ctx context.Context,
	attempts *specialistworkflow.AttemptBudget,
	query string,
) (websearch.CandidateReport, error) {
	receipt, err := specialistworkflow.RunAttempt(
		ctx, attempts, discoveryState{query: query}, machine.contracts.discovery,
	)
	observation, observed, observationErr := receipt.Observation()
	if observationErr != nil {
		return websearch.CandidateReport{}, observationErr
	}
	if err != nil {
		if observed {
			return cloneCandidateReport(observation.report), err
		}
		return websearch.CandidateReport{}, err
	}
	if !observed {
		return websearch.CandidateReport{}, fmt.Errorf("%w: discovery attempt returned no observation", ErrInvalidAcquisition)
	}
	if receipt.Verified() {
		return cloneCandidateReport(observation.report), nil
	}
	failure, failed, failureErr := receipt.Failure()
	if failureErr != nil {
		return cloneCandidateReport(observation.report), failureErr
	}
	if !failed || failure.kind != failureNoCandidates {
		return cloneCandidateReport(observation.report), fmt.Errorf("%w: discovery attempt returned no typed failure", ErrInvalidAcquisition)
	}
	return cloneCandidateReport(observation.report), websearch.ErrNoCandidates
}

func (machine *Machine) fetch(
	ctx context.Context,
	attempts *specialistworkflow.AttemptBudget,
	candidates []websearch.Candidate,
	result *Result,
) ([]websearch.Document, error) {
	if err := validateCandidateSliceBounds(candidates); err != nil {
		return nil, fmt.Errorf("%w: fetch input bounds: %v", ErrInvalidAcquisition, err)
	}
	receipt, err := specialistworkflow.RunAttempt(
		ctx, attempts, fetchState{candidates: cloneCandidates(candidates)}, machine.contracts.fetch,
	)
	recordFetchAttempt(result, attempts)
	observation, observed, observationErr := receipt.Observation()
	if observationErr != nil {
		return nil, observationErr
	}
	if observed {
		result.Fetches = append(result.Fetches, cloneDocumentReport(observation.report))
	}
	if err != nil {
		return nil, err
	}
	if !observed {
		return nil, fmt.Errorf("%w: fetch attempt returned no observation", ErrInvalidAcquisition)
	}
	if receipt.Verified() {
		return append([]websearch.Document{}, observation.report.Documents...), nil
	}
	failure, failed, failureErr := receipt.Failure()
	if failureErr != nil {
		return nil, failureErr
	}
	if !failed || failure.kind != failureNoDocuments {
		return nil, fmt.Errorf("%w: fetch attempt returned no typed failure", ErrInvalidAcquisition)
	}
	return nil, websearch.ErrNoDocuments
}

func recordDiscoveryAttempt(result *Result, attempts *specialistworkflow.AttemptBudget) {
	used := int(attempts.Used())
	if used > result.AcquisitionAttempts {
		result.DiscoveryAttempts += used - result.AcquisitionAttempts
		result.AcquisitionAttempts = used
	}
}

func recordFetchAttempt(result *Result, attempts *specialistworkflow.AttemptBudget) {
	used := int(attempts.Used())
	if used > result.AcquisitionAttempts {
		result.FetchAttempts += used - result.AcquisitionAttempts
		result.AcquisitionAttempts = used
	}
}

func everyProviderFailed(report websearch.CandidateReport) bool {
	if len(report.Diagnostics) == 0 {
		return false
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Outcome != websearch.DiscoveryFailed {
			return false
		}
	}
	return true
}

func isRecoverableEmptyFetch(report websearch.DocumentReport) bool {
	if len(report.Diagnostics) == 0 {
		return false
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Outcome == websearch.FetchEmpty {
			return true
		}
	}
	return false
}
