package contextcompiler

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func Compile(
	ctx context.Context,
	request Request,
	provider CandidateProvider,
	stations Stations,
) (Result, error) {
	result := Result{Context: assemblyline.ObjectiveContext{
		Capsules: []assemblyline.ObjectiveContextCapsule{},
	}}
	if ctx == nil {
		return result, fmt.Errorf("context compilation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if provider == nil {
		return result, fmt.Errorf("context compilation requires fixed retrieval authority")
	}
	termsInput := assemblyline.ContextSearchTermsInput{ExactInstruction: request.ExactInstruction}
	if _, err := assemblyline.NewContextSearchTermsJob(termsInput); err != nil {
		return result, err
	}
	retrievalConcepts, err := resolveRetrievalConcepts(ctx, request, termsInput, stations.Terms, &result)
	if err != nil {
		return result, err
	}

	set, err := provider.Retrieve(ctx, append([]string{}, retrievalConcepts...))
	if err != nil {
		return result, fmt.Errorf("retrieve fixed context candidates: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := validateCandidateSet(set); err != nil {
		return result, err
	}
	if len(retrievalConcepts) == 0 && len(set.Optional) != 0 {
		return result, fmt.Errorf("fixed context provider returned optional candidates for explicit empty search terms")
	}
	if set.Replan != nil {
		copy := *set.Replan
		result.Context.ReplanAuthority = &copy
	}

	selected := append([]assemblyline.ContextCandidateAuthority(nil), set.Required...)
	if len(retrievalConcepts) > 0 && len(set.Optional) > 0 {
		optional, relevanceCalls, err := selectRelevantAuthorities(
			ctx, request.ExactInstruction, retrievalConcepts, set.Optional, stations.Relevance,
		)
		if err != nil {
			return result, err
		}
		result.RelevanceCalls += relevanceCalls
		result.ModelCalls += relevanceCalls
		selected = append(selected, optional...)
	}
	if len(selected) == 0 {
		if err := result.Context.Validate(); err != nil {
			return result, err
		}
		return result, nil
	}
	content, minificationCalls, err := reduceSelectedAuthorities(
		ctx, request.ExactInstruction, selected, stations.Minification,
	)
	if err != nil {
		return result, err
	}
	result.MinificationCalls += minificationCalls
	result.ModelCalls += minificationCalls
	sources := make([]assemblyline.ObjectiveContextSource, len(selected))
	for index, authority := range selected {
		sources[index] = assemblyline.ObjectiveContextSource{
			Namespace: authority.Namespace, CandidateID: authority.CandidateID,
			ContentSHA256: authority.ContentSHA256,
		}
	}
	result.Context.Capsules = []assemblyline.ObjectiveContextCapsule{{
		Sources: sources, Content: content,
		ContentSHA256: assemblyline.ExactObjectiveContextSHA(content),
	}}
	if err := result.Context.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func resolveRetrievalConcepts(
	ctx context.Context,
	request Request,
	input assemblyline.ContextSearchTermsInput,
	station SearchTermsStation,
	result *Result,
) ([]string, error) {
	if request.Retrieval != nil {
		decision := assemblyline.ContextSearchTermsDecision{
			Schema: assemblyline.ContextSearchTermsSchemaV1,
			Terms:  append([]string{}, request.Retrieval.Concepts...),
		}
		if err := decision.ValidateFor(input); err != nil {
			return nil, fmt.Errorf("code-owned retrieval directive: %w", err)
		}
		return canonicalRetrievalConcepts(decision.Terms), nil
	}
	if station == nil {
		return nil, fmt.Errorf("context search terms remain unresolved but the station is unavailable")
	}
	terms, receipt, err := station.Generate(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("context search terms: %w", err)
	}
	if err := validateReceipt("context search terms", receipt); err != nil {
		return nil, err
	}
	if err := terms.ValidateFor(input); err != nil {
		return nil, err
	}
	result.SearchTermsCalls += receipt.Calls
	result.ModelCalls += receipt.Calls
	return canonicalRetrievalConcepts(terms.Terms), nil
}

// canonicalRetrievalConcepts removes model-authored casing and array order
// once, before the same immutable semantic leaves are given to fixed retrieval
// and bounded relevance selection.
func canonicalRetrievalConcepts(terms []string) []string {
	concepts := make([]string, len(terms))
	for index, term := range terms {
		concepts[index] = strings.ToLower(term)
	}
	sort.Strings(concepts)
	return concepts
}

func validateReceipt(label string, receipt StationReceipt) error {
	if receipt.Calls < 1 || receipt.Calls > maxStationAttempts {
		return fmt.Errorf("%s reported %d calls outside the bounded correction budget", label, receipt.Calls)
	}
	return nil
}

func validateCandidateSet(set CandidateSet) error {
	seenIDs := make(map[string]struct{}, len(set.Required)+len(set.Optional))
	seenContent := make(map[string]struct{}, len(set.Required)+len(set.Optional))
	replanCandidates := 0
	for index, authority := range append(
		append([]assemblyline.ContextCandidateAuthority(nil), set.Required...), set.Optional...,
	) {
		validated, err := assemblyline.NewContextCandidateAuthority(
			authority.Namespace, authority.CandidateID, authority.Content,
		)
		if err != nil {
			return fmt.Errorf("context candidate %d: %w", index, err)
		}
		if validated.ContentSHA256 != authority.ContentSHA256 {
			return fmt.Errorf("context candidate %s content hash does not match", authority.CandidateID)
		}
		if _, duplicate := seenIDs[authority.CandidateID]; duplicate {
			return fmt.Errorf("context candidate ID %q is duplicated across provider sets", authority.CandidateID)
		}
		seenIDs[authority.CandidateID] = struct{}{}
		if _, duplicate := seenContent[authority.ContentSHA256]; duplicate {
			return fmt.Errorf("context candidate %q duplicates exact provider content", authority.CandidateID)
		}
		seenContent[authority.ContentSHA256] = struct{}{}
		if authority.Namespace == "objective_replan" {
			if index >= len(set.Required) {
				return fmt.Errorf("objective replan context must be required")
			}
			if set.Replan == nil || authority.Content != set.Replan.Feedback ||
				authority.ContentSHA256 != set.Replan.FeedbackSHA256 {
				return fmt.Errorf("objective replan candidate differs from exact replan authority")
			}
			replanCandidates++
		}
	}
	if set.Replan != nil && replanCandidates != 1 {
		return fmt.Errorf("objective replan authority requires exactly one required context candidate")
	}
	if set.Replan == nil && replanCandidates != 0 {
		return fmt.Errorf("objective replan candidate has no exact replan authority")
	}
	context := assemblyline.ObjectiveContext{
		Capsules: []assemblyline.ObjectiveContextCapsule{}, ReplanAuthority: set.Replan,
	}
	return context.Validate()
}

func selectedInAuthorityOrder(
	authorities []assemblyline.ContextCandidateAuthority,
	ids []string,
) []assemblyline.ContextCandidateAuthority {
	requested := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}
	selected := make([]assemblyline.ContextCandidateAuthority, 0, len(ids))
	for _, authority := range authorities {
		if _, keep := requested[authority.CandidateID]; keep {
			selected = append(selected, authority)
		}
	}
	return selected
}
