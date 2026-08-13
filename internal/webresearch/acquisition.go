package webresearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialistworkflow"
	"github.com/gryph/omnidex/internal/websearch"
)

func (machine *Machine) initialAcquisition(
	ctx context.Context,
	attempts *specialistworkflow.AttemptBudget,
	result *Result,
) ([]websearch.Candidate, bool, error) {
	if machine.objective.InitialQuery == "" {
		return nil, false, nil
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

func (machine *Machine) resolveSearchTerms(
	ctx context.Context,
	attempts *specialistworkflow.AttemptBudget,
	result *Result,
) ([]websearch.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	call := SearchTermsCall{
		Question:         machine.objective.Question,
		Context:          assemblyline.CloneObjectiveContext(machine.objective.Context),
		AttemptedQueries: attemptedQueries(machine.objective.InitialQuery),
		MaxTerms:         machine.config.MaxSearchTerms,
		MaxTermBytes:     machine.config.MaxSearchTermBytes,
	}
	decision, err := machine.terms.Resolve(ctx, cloneSearchTermsCall(call))
	result.SearchTermsCalls++
	if err != nil {
		return nil, fmt.Errorf("search terms station: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	terms, err := validateSearchTerms(decision, call.AttemptedQueries, machine.config)
	if err != nil {
		return nil, err
	}
	result.Steps = append(result.Steps, StepSearchTermsResolved)
	all := make([]websearch.Candidate, 0, machine.config.MaxFetchCandidates)
	for _, term := range terms {
		report, discoverErr := machine.discover(ctx, attempts, term)
		recordDiscoveryAttempt(result, attempts)
		result.Discovery = append(result.Discovery, cloneCandidateReport(report))
		if discoverErr != nil && !errors.Is(discoverErr, websearch.ErrNoCandidates) {
			return nil, fmt.Errorf("expanded discovery for %q: %w", term, discoverErr)
		}
		if discoverErr == nil {
			all = append(all, report.Candidates...)
		}
	}
	result.Steps = append(result.Steps, StepExpandedDiscovery)
	merged, err := reduceCandidates(all, machine.config.MaxFetchCandidates)
	if err != nil {
		return nil, err
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("%w after %d bounded search terms", ErrEvidenceUnavailable, len(terms))
	}
	return merged, nil
}

func attemptedQueries(initial string) []string {
	if initial == "" {
		return []string{}
	}
	return []string{initial}
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

func validateSearchTerms(decision SearchTermsDecision, attempted []string, config Config) ([]string, error) {
	if len(decision.Terms) < 1 || len(decision.Terms) > config.MaxSearchTerms {
		return nil, fmt.Errorf("%w: expected 1..%d terms", ErrInvalidSearchTerms, config.MaxSearchTerms)
	}
	seen := make(map[string]struct{}, len(decision.Terms)+len(attempted))
	for _, query := range attempted {
		seen[strings.ToLower(query)] = struct{}{}
	}
	terms := make([]string, len(decision.Terms))
	for index, term := range decision.Terms {
		if term == "" || term != strings.TrimSpace(term) || len(term) > config.MaxSearchTermBytes {
			return nil, fmt.Errorf("%w: term %d must be trimmed and contain 1..%d bytes", ErrInvalidSearchTerms, index, config.MaxSearchTermBytes)
		}
		if !utf8.ValidString(term) || strings.ContainsRune(term, '\x00') || strings.ContainsAny(term, "\r\n") {
			return nil, fmt.Errorf("%w: term %d must be one UTF-8 line without NUL", ErrInvalidSearchTerms, index)
		}
		identity := strings.ToLower(term)
		if _, duplicate := seen[identity]; duplicate {
			return nil, fmt.Errorf("%w: duplicate search term %q", ErrInvalidSearchTerms, term)
		}
		seen[identity] = struct{}{}
		terms[index] = term
	}
	return terms, nil
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
