package webresearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/websearch"
)

func (machine *evidenceMachine) initialAcquisition(ctx context.Context, result *evidenceRun) ([]websearch.Candidate, error) {
	query := machine.objective.InitialQuery
	if err := validateAcquisitionQuery(query); err != nil {
		return nil, fmt.Errorf("%w: initial query: %v", ErrInvalidObjective, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report, err := machine.acquisition.Discover(ctx, websearch.QueryRequest{Query: query})
	if boundsErr := validateCandidateReportBounds(report); boundsErr != nil {
		return nil, errors.Join(fmt.Errorf("%w: discovery report bounds: %v", ErrInvalidAcquisition, boundsErr), err, ctx.Err())
	}
	// Retain bounded observations even when acquisition fails. They are not
	// selected evidence and cannot authorize a subsequent semantic call.
	report = cloneCandidateReport(report)
	result.Discovery = append(result.Discovery, report)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, errors.Join(err, contextErr)
	}
	if err != nil && !errors.Is(err, websearch.ErrNoCandidates) {
		return nil, fmt.Errorf("initial discovery: %w", err)
	}
	if validationErr := validateCandidateReport(report, query, err); validationErr != nil {
		return nil, errors.Join(validationErr, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: initial discovery: %w", ErrEvidenceUnavailable, err)
	}
	return reduceCandidates(report.Candidates, machine.config.MaxFetchCandidates)
}

func (machine *evidenceMachine) fetch(ctx context.Context, candidates []websearch.Candidate, result *evidenceRun) ([]websearch.Document, error) {
	if err := validateCandidateSliceBounds(candidates); err != nil {
		return nil, fmt.Errorf("%w: fetch input bounds: %v", ErrInvalidAcquisition, err)
	}
	if err := validateFetchCandidates(candidates, machine.config.MaxFetchCandidates); err != nil {
		return nil, fmt.Errorf("%w: fetch input: %v", ErrInvalidAcquisition, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids := make([]websearch.CandidateID, len(candidates))
	for index, candidate := range candidates {
		ids[index] = candidate.ID
	}
	report, err := machine.acquisition.Fetch(ctx, websearch.FetchRequest{
		Candidates: cloneCandidates(candidates), CandidateIDs: ids,
	})
	if boundsErr := validateDocumentReportBounds(report); boundsErr != nil {
		return nil, errors.Join(fmt.Errorf("%w: fetch report bounds: %v", ErrInvalidAcquisition, boundsErr), err, ctx.Err())
	}
	report = cloneDocumentReport(report)
	result.Fetches = append(result.Fetches, report)
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, errors.Join(err, contextErr)
	}
	if err != nil && !errors.Is(err, websearch.ErrNoDocuments) {
		return nil, fmt.Errorf("document fetch: %w", err)
	}
	if validationErr := validateDocumentReport(report, candidates, err); validationErr != nil {
		return nil, errors.Join(validationErr, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: document fetch: %w", ErrEvidenceUnavailable, err)
	}
	return report.Documents, nil
}
