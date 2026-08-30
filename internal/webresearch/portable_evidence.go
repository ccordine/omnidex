package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/websearch"
)

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
	var ledger SemanticCallLedger
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
		relation, receipt, err := runPortableSemanticLeaf(
			ctx, stations, job,
			func(raw string) (assemblyline.WebRelevanceRelationDecision, error) {
				return assemblyline.DecodeWebRelevanceRelationLeaf(input, raw)
			},
		)
		if err != nil {
			return RelevanceDecision{}, err
		}
		if err := ledger.Record(
			"web relevance candidate "+candidate.CandidateID,
			receipt,
			exactPortableSemanticLeafCalls,
		); err != nil {
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
	receipt, err := ledger.ValidateForMaximum(
		"web relevance station", len(candidates)*exactPortableSemanticLeafCalls,
	)
	if err != nil {
		return RelevanceDecision{}, err
	}
	return RelevanceDecision{
		Outcome: outcome, CandidateIDs: ids, SemanticCalls: receipt.Calls,
		CallLedger: ledger.Clone(),
	}, nil
}
