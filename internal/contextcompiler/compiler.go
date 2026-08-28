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
	termsInput := assemblyline.ContextSearchTermsInput{
		ExactInstruction: request.ExactInstruction,
		Scope:            request.Scope,
	}
	if _, err := assemblyline.NewContextSearchTermCoverageJob(
		assemblyline.ContextSearchTermLeafInput{
			ExactInstruction: termsInput.ExactInstruction, Scope: termsInput.Scope,
			AcceptedTerms: []string{},
		},
	); err != nil {
		return result, err
	}
	if request.Retrieval == nil {
		availability, err := provider.SearchAvailability(ctx)
		if err != nil {
			return result, fmt.Errorf("inspect fixed context search availability: %w", err)
		}
		directive, calls, err := ResolveRetrievalDirective(
			ctx, request.ExactInstruction, request.Scope, availability, stations.Terms,
		)
		if err != nil {
			return result, err
		}
		request.Retrieval = &directive
		result.SearchTermsCalls += calls
		result.ModelCalls += calls
	}
	retrievalConcepts, err := resolveRetrievalConcepts(request, termsInput)
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
	if set.Replan != nil {
		copy := *set.Replan
		result.Context.ReplanAuthority = &copy
	}

	selected := append([]assemblyline.ContextCandidateAuthority(nil), set.Required...)
	if len(set.Optional) > 0 {
		optional, relevanceCalls, err := selectRelevantAuthorities(
			ctx, request.ExactInstruction, request.Scope, retrievalConcepts,
			set.Optional, stations.Relevance,
		)
		if err != nil {
			return result, err
		}
		optional = expandOptionalSelectionGroups(
			set.Optional, optional, set.OptionalSelectionGroups,
		)
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
		ctx, request.ExactInstruction, request.Scope, selected, stations.Minification,
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
	request Request,
	input assemblyline.ContextSearchTermsInput,
) ([]string, error) {
	if request.Retrieval == nil {
		return nil, fmt.Errorf("context retrieval directive was not resolved before acquisition")
	}
	decision := assemblyline.ContextSearchTermsDecision{
		Schema: assemblyline.ContextSearchTermsSchemaV1,
		Terms:  append([]string{}, request.Retrieval.Concepts...),
	}
	if err := decision.ValidateFor(input); err != nil {
		return nil, fmt.Errorf("code-owned retrieval directive: %w", err)
	}
	return canonicalRetrievalConcepts(decision.Terms), nil
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
	if receipt.Reused {
		if receipt.Calls != 0 {
			return fmt.Errorf("%s reuse reported %d provider calls", label, receipt.Calls)
		}
		return nil
	}
	if receipt.Calls < 1 || receipt.Calls > assemblyline.MaxSemanticStationAttempts {
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
	if err := validateOptionalSelectionGroups(set); err != nil {
		return err
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
