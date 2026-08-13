package webresearch

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialistworkflow"
	"github.com/gryph/omnidex/internal/websearch"
)

type Machine struct {
	objective  Objective
	config     Config
	terms      SearchTermsStation
	relevance  RelevanceStation
	synthesis  GroundedSynthesisStation
	correction GroundedSynthesisCorrectionStation
	review     ClaimEvidenceReviewStation
	contracts  acquisitionContracts
}

func New(
	objective Objective,
	config Config,
	acquisition Acquisition,
	terms SearchTermsStation,
	relevance RelevanceStation,
	synthesis GroundedSynthesisStation,
	correction GroundedSynthesisCorrectionStation,
	review ClaimEvidenceReviewStation,
) (*Machine, error) {
	if err := validateObjective(objective); err != nil {
		return nil, err
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	if nilInterface(acquisition) || nilInterface(terms) || nilInterface(relevance) ||
		nilInterface(synthesis) || nilInterface(correction) || nilInterface(review) {
		return nil, fmt.Errorf("%w: acquisition and all exact stations are required", ErrInvalidConfiguration)
	}
	limits := acquisition.Limits()
	if limits.MaxDocuments < 1 || limits.MaxDocuments > 32 ||
		config.MaxFetchCandidates > limits.MaxDocuments {
		return nil, fmt.Errorf(
			"%w: workflow fetch bound %d exceeds deterministic acquisition bound %d",
			ErrInvalidConfiguration, config.MaxFetchCandidates, limits.MaxDocuments,
		)
	}
	contracts, err := newAcquisitionContracts(acquisition, config.MaxFetchCandidates)
	if err != nil {
		return nil, fmt.Errorf("%w: acquisition contracts: %w", ErrInvalidConfiguration, err)
	}
	objective.Acceptance = append([]AcceptancePredicate{}, objective.Acceptance...)
	return &Machine{
		objective: objective, config: config,
		terms: terms, relevance: relevance, synthesis: synthesis, correction: correction,
		review: review, contracts: contracts,
	}, nil
}

func (machine *Machine) Run(ctx context.Context) (Result, error) {
	if ctx == nil {
		return Result{}, ErrNilContext
	}
	if machine == nil {
		return Result{}, fmt.Errorf("%w: machine is nil", ErrInvalidConfiguration)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	attemptLimit := uint16(machine.config.MaxSearchTerms + 3)
	result := Result{Objective: cloneObjective(machine.objective), AcquisitionAttemptLimit: int(attemptLimit)}
	fail := func(runErr error) (Result, error) {
		if contextErr := ctx.Err(); contextErr != nil {
			return Result{}, contextErr
		}
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return Result{}, runErr
		}
		cloned := cloneResult(result)
		if contextErr := ctx.Err(); contextErr != nil {
			return Result{}, contextErr
		}
		return cloned, runErr
	}
	attempts, err := specialistworkflow.NewAttemptBudget(attemptLimit)
	if err != nil {
		return fail(fmt.Errorf("%w: acquisition attempt budget: %w", ErrInvalidConfiguration, err))
	}
	candidates, initialUsable, err := machine.initialAcquisition(ctx, attempts, &result)
	if err != nil {
		return fail(err)
	}
	var documents []websearch.Document
	if initialUsable {
		documents, err = machine.fetch(ctx, attempts, candidates, &result)
		if err == nil {
			result.Steps = append(result.Steps, StepDocumentsFetched)
		} else if !errors.Is(err, websearch.ErrNoDocuments) || !isRecoverableEmptyFetch(result.Fetches[len(result.Fetches)-1]) {
			return fail(err)
		}
	}
	if len(documents) == 0 {
		candidates, err = machine.resolveSearchTerms(ctx, attempts, &result)
		if err != nil {
			return fail(err)
		}
		documents, err = machine.fetch(ctx, attempts, candidates, &result)
		if err != nil {
			return fail(fmt.Errorf("%w after bounded search-term resolution: %w", ErrEvidenceUnavailable, err))
		}
		result.Steps = append(result.Steps, StepDocumentsFetched)
	}
	var evidence []Evidence
	var projected []ProjectedEvidence
	for {
		evidence = evidenceFromDocuments(documents)
		result.Evidence = cloneEvidence(evidence)
		var relevant bool
		projected, relevant, err = machine.selectAndProject(ctx, evidence, &result)
		if err != nil {
			return fail(err)
		}
		if relevant {
			break
		}
		if result.SearchTermsCalls > 0 {
			return fail(fmt.Errorf("%w after bounded relevance and search-term resolution", ErrEvidenceUnavailable))
		}
		candidates, err = machine.resolveSearchTerms(ctx, attempts, &result)
		if err != nil {
			return fail(err)
		}
		documents, err = machine.fetch(ctx, attempts, candidates, &result)
		if err != nil {
			return fail(fmt.Errorf("%w after bounded relevance search-term resolution: %w", ErrEvidenceUnavailable, err))
		}
		result.Steps = append(result.Steps, StepDocumentsFetched)
	}
	result.Projected = cloneProjection(projected)
	result.Steps = append(result.Steps, StepEvidenceProjected)
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	decision, err := machine.synthesis.Synthesize(ctx, GroundedSynthesisCall{
		Question:          machine.objective.Question,
		Context:           assemblyline.CloneObjectiveContext(machine.objective.Context),
		Evidence:          cloneProjection(projected),
		MaxParagraphs:     machine.config.MaxSynthesisParagraphs,
		MaxParagraphBytes: machine.config.MaxSynthesisParagraphBytes,
	})
	result.SynthesisCalls++
	if err != nil {
		return fail(fmt.Errorf("grounded synthesis station: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	artifact, err := buildArtifact(decision, projected, evidence, machine.config)
	if err != nil {
		return fail(err)
	}
	result.Steps = append(result.Steps, StepSynthesisResolved)
	issue, err := machine.reviewSynthesis(ctx, artifact.Paragraphs, projected, &result)
	if err != nil {
		return fail(err)
	}
	if issue != nil {
		corrected, correctionErr := machine.correctSynthesis(ctx, artifact.Paragraphs, projected, *issue, &result)
		if correctionErr != nil {
			return fail(correctionErr)
		}
		artifact, err = buildArtifact(GroundedSynthesisDecision{Paragraphs: corrected}, projected, evidence, machine.config)
		if err != nil {
			return fail(err)
		}
		result.Steps = append(result.Steps, StepSynthesisCorrected)
		secondIssue, reviewErr := machine.reviewSynthesis(ctx, artifact.Paragraphs, projected, &result)
		if reviewErr != nil {
			return fail(reviewErr)
		}
		if secondIssue != nil {
			return fail(claimEvidenceIssueError(*secondIssue))
		}
	}
	if err := commitReviewedCompletion(ctx, &result, artifact); err != nil {
		return fail(err)
	}
	return cloneResult(result), nil
}
