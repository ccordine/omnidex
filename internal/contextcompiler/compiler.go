package contextcompiler

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
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
	if request.KnownArtifactPaths == nil {
		return result, fmt.Errorf(
			"context compilation requires explicit current-artifact provenance, including an empty set",
		)
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(request.KnownArtifactPaths)
	if err != nil {
		return result, fmt.Errorf("context compilation current-artifact provenance: %w", err)
	}
	if err := validateRetrievalAuthority(request.ExactInstruction, request.Scope); err != nil {
		return result, err
	}
	if err := validateRetrievalAuthority(request.ModelInstruction, request.Scope); err != nil {
		return result, fmt.Errorf("context compilation model instruction: %w", err)
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"context compilation model instruction", provenance, request.ModelInstruction,
	); err != nil {
		return result, err
	}
	if request.Retrieval == nil {
		availability, err := provider.SearchAvailability(ctx)
		if err != nil {
			return result, fmt.Errorf("inspect fixed context search availability: %w", err)
		}
		directive, err := ResolveRetrievalDirective(
			ctx, request.ExactInstruction, request.Scope, availability,
		)
		if err != nil {
			return result, err
		}
		request.Retrieval = &directive
	}
	retrievalQueries, err := retrievalQueries(
		request.ExactInstruction, request.Scope, *request.Retrieval,
	)
	if err != nil {
		return result, err
	}

	set, err := provider.Retrieve(ctx, append([]string{}, retrievalQueries...))
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
			ctx, request.ModelInstruction, request.Scope,
			request.KnownArtifactPaths, set.Optional, stations.Relevance,
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
		ctx, request.ModelInstruction, request.Scope, request.KnownArtifactPaths,
		selected, stations.Minification,
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

func validateReceipt(label string, receipt StationReceipt) error {
	if receipt.Reused {
		if receipt.Calls != 0 {
			return fmt.Errorf("%s reuse reported %d provider calls", label, receipt.Calls)
		}
		return nil
	}
	if receipt.Calls != assemblyline.ExactSemanticLeafCalls {
		return fmt.Errorf(
			"%s reported %d calls; one raw semantic leaf requires exactly %d",
			label, receipt.Calls, assemblyline.ExactSemanticLeafCalls,
		)
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
