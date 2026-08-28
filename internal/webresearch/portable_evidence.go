package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

func (stations *PortableStations) Resolve(
	ctx context.Context,
	call SearchTermsCall,
) (SearchTermsDecision, error) {
	if err := validatePortableSearchTermsCall(call); err != nil {
		return SearchTermsDecision{}, err
	}
	base := assemblyline.WebSearchTermsInput{
		ExactQuestion:    call.Question,
		Context:          assemblyline.CloneObjectiveContext(call.Context),
		AttemptedQueries: call.AttemptedQueries,
		MaxTerms:         call.MaxTerms, MaxTermBytes: call.MaxTermBytes,
	}
	accepted := make([]string, 0, call.MaxTerms)
	semanticCalls := 0
	for {
		leafInput := webSearchTermLeafInput(base, accepted)
		if len(accepted) > 0 {
			coverageJob, err := assemblyline.NewWebSearchTermCoverageJob(leafInput)
			if err != nil {
				return SearchTermsDecision{}, fmt.Errorf("build web search term coverage job: %w", err)
			}
			coverage, err := runPortableSemanticLeaf(
				ctx, stations, coverageJob,
				func(raw string) (assemblyline.WebSearchTermCoverageDecision, error) {
					return assemblyline.DecodeWebSearchTermCoverageLeaf(leafInput, raw)
				},
			)
			semanticCalls++
			if err != nil {
				return SearchTermsDecision{}, err
			}
			if coverage.Coverage == assemblyline.WebNoUncoveredQueryTerm {
				decision, err := assemblyline.AssembleWebSearchTermsDecision(base, accepted)
				if err != nil {
					return SearchTermsDecision{}, err
				}
				return SearchTermsDecision{
					Terms: append([]string{}, decision.Terms...), SemanticCalls: semanticCalls,
				}, nil
			}
		}
		if len(accepted) >= call.MaxTerms {
			return SearchTermsDecision{}, fmt.Errorf(
				"web search terms still require another term after the %d-term bound",
				call.MaxTerms,
			)
		}

		termJob, err := assemblyline.NewWebSearchTermJob(leafInput)
		if err != nil {
			return SearchTermsDecision{}, fmt.Errorf("build web search term job: %w", err)
		}
		term, err := runPortableSemanticLeaf(
			ctx, stations, termJob,
			func(raw string) (assemblyline.WebSearchTermDecision, error) {
				return assemblyline.DecodeWebSearchTermLeaf(leafInput, raw)
			},
		)
		semanticCalls++
		if err != nil {
			return SearchTermsDecision{}, err
		}
		accepted = append(accepted, term.Term)
	}
}

func webSearchTermLeafInput(
	base assemblyline.WebSearchTermsInput,
	accepted []string,
) assemblyline.WebSearchTermLeafInput {
	return assemblyline.WebSearchTermLeafInput{
		ExactQuestion:    base.ExactQuestion,
		Context:          assemblyline.CloneObjectiveContext(base.Context),
		AttemptedQueries: append([]string{}, base.AttemptedQueries...),
		AcceptedTerms:    append([]string{}, accepted...),
		MaxTerms:         base.MaxTerms,
		MaxTermBytes:     base.MaxTermBytes,
	}
}

func (stations *PortableStations) Select(
	ctx context.Context,
	call RelevanceCall,
) (RelevanceDecision, error) {
	if err := validatePortableRelevanceCall(call); err != nil {
		return RelevanceDecision{}, err
	}
	candidates := make([]assemblyline.WebRelevanceCandidate, len(call.Candidates))
	for index, candidate := range call.Candidates {
		candidates[index] = assemblyline.WebRelevanceCandidate{
			CandidateID: string(candidate.CandidateID), Title: candidate.Title,
			Snippet: candidate.Snippet, Excerpt: candidate.Excerpt,
		}
	}
	base := assemblyline.WebRelevanceInput{
		ExactQuestion: call.Question,
		Context:       assemblyline.CloneObjectiveContext(call.Context),
		Candidates:    candidates, MaxSelections: call.MaxSelections,
	}
	selected := make([]string, 0, call.MaxSelections)
	semanticCalls := 0
	for _, candidate := range candidates {
		if len(selected) == call.MaxSelections {
			break
		}
		input := assemblyline.WebRelevanceRelationInput{
			ExactQuestion: base.ExactQuestion,
			Context:       assemblyline.CloneObjectiveContext(base.Context),
			Candidate:     candidate,
		}
		job, err := assemblyline.NewWebRelevanceRelationJob(input)
		if err != nil {
			return RelevanceDecision{}, fmt.Errorf("build web relevance relation job: %w", err)
		}
		relation, err := runPortableSemanticLeaf(
			ctx, stations, job,
			func(raw string) (assemblyline.WebRelevanceRelationDecision, error) {
				return assemblyline.DecodeWebRelevanceRelationLeaf(input, raw)
			},
		)
		semanticCalls++
		if err != nil {
			return RelevanceDecision{}, err
		}
		if relation.Relation == assemblyline.WebCandidateRelevant {
			selected = append(selected, candidate.CandidateID)
		}
	}
	decision, err := assemblyline.AssembleWebRelevanceDecision(base, selected)
	if err != nil {
		return RelevanceDecision{}, err
	}
	ids := make([]websearch.CandidateID, len(decision.CandidateIDs))
	for index, id := range decision.CandidateIDs {
		ids[index] = websearch.CandidateID(id)
	}
	outcome := RelevanceNone
	if len(ids) > 0 {
		outcome = RelevanceSelected
	}
	return RelevanceDecision{
		Outcome: outcome, CandidateIDs: ids, SemanticCalls: semanticCalls,
	}, nil
}
