package contextcompiler

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func selectRelevantAuthorities(
	ctx context.Context,
	modelInstruction string,
	scope assemblyline.ContextScope,
	knownArtifactPaths []string,
	authorities []assemblyline.ContextCandidateAuthority,
	station RelevanceStation,
) ([]assemblyline.ContextCandidateAuthority, int, error) {
	if len(authorities) == 0 {
		return []assemblyline.ContextCandidateAuthority{}, 0, nil
	}
	if station == nil {
		return nil, 0, fmt.Errorf("context relevance remains unresolved but the station is unavailable")
	}
	selected := make([]assemblyline.ContextCandidateAuthority, 0, len(authorities))
	calls := 0
	for candidateIndex, candidate := range authorities {
		input := assemblyline.ContextRelevanceRelationInput{
			ExactInstruction:   modelInstruction,
			Candidate:          candidate,
			Scope:              scope,
			KnownArtifactPaths: append([]string{}, knownArtifactPaths...),
		}
		if _, err := assemblyline.NewContextRelevanceRelationJob(input); err != nil {
			return nil, calls, fmt.Errorf(
				"context relevance candidate %d: %w", candidateIndex+1, err,
			)
		}
		relation, leafCalls, err := station.Relate(ctx, input)
		calls += leafCalls
		if err != nil {
			return nil, calls, fmt.Errorf(
				"context relevance candidate %d: %w", candidateIndex+1, err,
			)
		}
		if err := validateCallCount("context relevance relation", leafCalls); err != nil {
			return nil, calls, err
		}
		if err := relation.ValidateFor(input); err != nil {
			return nil, calls, err
		}
		if relation.Relation == assemblyline.ContextCandidateDirectlyRelevant {
			selected = append(selected, candidate)
		}
	}
	return selected, calls, nil
}

func reduceSelectedAuthorities(
	ctx context.Context,
	modelInstruction string,
	scope assemblyline.ContextScope,
	knownArtifactPaths []string,
	selected []assemblyline.ContextCandidateAuthority,
	station MinificationStation,
) (string, int, error) {
	verbatim := joinAuthorityContent(selected)
	if len(verbatim) <= assemblyline.ContextMinificationThresholdBytes {
		return verbatim, 0, nil
	}
	if station == nil {
		return "", 0, fmt.Errorf("context exceeds the verbatim target but the minification station is unavailable")
	}

	groups, err := partitionContextAuthorities(selected,
		assemblyline.MaxContextMinificationAuthorities,
		assemblyline.MaxContextMinificationProjectionBytes)
	if err != nil {
		return "", 0, fmt.Errorf("partition context minification input: %w", err)
	}
	contents := make([]string, 0, len(groups))
	seenContent := make(map[string]struct{}, len(groups))
	totalCalls := 0
	for groupIndex, group := range groups {
		pending := append([]string(nil), contents...)
		for _, remaining := range groups[groupIndex:] {
			pending = append(pending, joinAuthorityContent(remaining))
		}
		if content := strings.Join(pending, "\n\n"); len(content) <= assemblyline.ContextMinificationThresholdBytes {
			return content, totalCalls, nil
		}
		content := joinAuthorityContent(group)
		if len(group) > 1 {
			input := assemblyline.ContextMinificationInput{
				ExactInstruction:    modelInstruction,
				SelectedAuthorities: append([]assemblyline.ContextCandidateAuthority(nil), group...),
				Scope:               scope,
				KnownArtifactPaths:  append([]string{}, knownArtifactPaths...),
			}
			if _, err := assemblyline.NewContextMinificationJob(input); err != nil {
				return "", totalCalls, fmt.Errorf("context minification group %d: %w", groupIndex+1, err)
			}
			decision, leafCalls, err := station.Minify(ctx, input)
			totalCalls += leafCalls
			if err != nil {
				return "", totalCalls, fmt.Errorf("context minification group %d: %w", groupIndex+1, err)
			}
			if err := validateCallCount("context minification", leafCalls); err != nil {
				return "", totalCalls, err
			}
			if err := decision.ValidateFor(input); err != nil {
				return "", totalCalls, err
			}
			content = decision.MinimalContext
		}
		if _, duplicate := seenContent[content]; duplicate {
			continue
		}
		seenContent[content] = struct{}{}
		contents = append(contents, content)
	}
	// Minification preserves the necessary result, even when that information
	// cannot fit a guessed byte target. Never repeatedly squeeze accepted facts
	// into a smaller artificial window or discard a completed semantic result.
	return strings.Join(contents, "\n\n"), totalCalls, nil
}

func partitionContextAuthorities(
	authorities []assemblyline.ContextCandidateAuthority,
	maximumCount int,
	maximumBytes int,
) ([][]assemblyline.ContextCandidateAuthority, error) {
	if maximumCount < 1 || maximumBytes < 1 {
		return nil, fmt.Errorf("context partition requires positive count and byte budgets")
	}
	groups := make([][]assemblyline.ContextCandidateAuthority, 0)
	current := make([]assemblyline.ContextCandidateAuthority, 0, maximumCount)
	currentBytes := 0
	for _, authority := range authorities {
		entryBytes := contextAuthorityProjectionBytes(authority)

		if len(current) > 0 && (len(current) == maximumCount || currentBytes+entryBytes > maximumBytes) {
			groups = append(groups, current)
			current = make([]assemblyline.ContextCandidateAuthority, 0, maximumCount)
			currentBytes = 0
		}
		current = append(current, authority)
		currentBytes += entryBytes
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups, nil
}

func contextAuthorityProjectionBytes(authority assemblyline.ContextCandidateAuthority) int {
	return len(authority.Namespace) + len(authority.CandidateID) +
		len(authority.Content)
}

func joinAuthorityContent(authorities []assemblyline.ContextCandidateAuthority) string {
	contents := make([]string, len(authorities))
	for index, authority := range authorities {
		contents[index] = authority.Content
	}
	return strings.Join(contents, "\n\n")
}

// ReduceRelevant minifies already scoped context using the same bounded grouping
// and completion rules as Compile, without performing another relevance pass.
func ReduceRelevant(ctx context.Context, instruction string, selected []assemblyline.ContextCandidateAuthority, station MinificationStation) (string, error) {
	content, _, err := reduceSelectedAuthorities(ctx, instruction, "", []string{}, selected, station)
	return content, err
}
