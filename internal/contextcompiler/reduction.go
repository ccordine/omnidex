package contextcompiler

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func selectRelevantAuthorities(
	ctx context.Context,
	exactInstruction string,
	retrievalConcepts []string,
	authorities []assemblyline.ContextCandidateAuthority,
	station RelevanceStation,
) ([]assemblyline.ContextCandidateAuthority, int, error) {
	if len(authorities) == 0 {
		return []assemblyline.ContextCandidateAuthority{}, 0, nil
	}
	if station == nil {
		return nil, 0, fmt.Errorf("context relevance remains unresolved but the station is unavailable")
	}
	pages, err := partitionContextAuthorities(
		authorities,
		assemblyline.MaxContextCandidateAuthorities,
		assemblyline.MaxContextCandidateProjectionBytes,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("partition context relevance candidates: %w", err)
	}
	selected := make([]assemblyline.ContextCandidateAuthority, 0, len(authorities))
	calls := 0
	for pageIndex, page := range pages {
		input := assemblyline.ContextRelevanceInput{
			ExactInstruction:     exactInstruction,
			RetrievalConcepts:    append([]string{}, retrievalConcepts...),
			CandidateAuthorities: append([]assemblyline.ContextCandidateAuthority(nil), page...),
			MaxSelections:        min(assemblyline.MaxContextRelevanceSelections, len(page)),
		}
		if _, err := assemblyline.NewContextRelevanceJob(input); err != nil {
			return nil, calls, fmt.Errorf("context relevance page %d: %w", pageIndex+1, err)
		}
		decision, receipt, err := station.SelectRelevant(ctx, input)
		if err != nil {
			return nil, calls, fmt.Errorf("context relevance page %d: %w", pageIndex+1, err)
		}
		if err := validateReceipt("context relevance", receipt); err != nil {
			return nil, calls, err
		}
		calls += receipt.Calls
		if err := decision.ValidateFor(input); err != nil {
			return nil, calls, err
		}
		selected = append(selected, selectedInAuthorityOrder(
			page, decision.ReferencedCandidateIDs,
		)...)
	}
	return selected, calls, nil
}

func reduceSelectedAuthorities(
	ctx context.Context,
	exactInstruction string,
	selected []assemblyline.ContextCandidateAuthority,
	station MinificationStation,
) (string, int, error) {
	verbatim := joinAuthorityContent(selected)
	if len(verbatim) <= assemblyline.MaxContextMinifiedBytes {
		return verbatim, 0, nil
	}
	if station == nil {
		return "", 0, fmt.Errorf("context exceeds the verbatim target but the minification station is unavailable")
	}

	current := append([]assemblyline.ContextCandidateAuthority(nil), selected...)
	previousBytes := len(verbatim)
	totalCalls := 0
	for len(current) > 0 {
		groups, err := partitionContextAuthorities(
			current,
			assemblyline.MaxContextMinificationAuthorities,
			assemblyline.MaxContextMinificationProjectionBytes,
		)
		if err != nil {
			return "", totalCalls, fmt.Errorf("partition context minification input: %w", err)
		}
		next := make([]assemblyline.ContextCandidateAuthority, 0, len(groups))
		seenContent := make(map[string]struct{}, len(groups))
		for groupIndex, group := range groups {
			content := joinAuthorityContent(group)
			if len(group) > 1 {
				input := assemblyline.ContextMinificationInput{
					ExactInstruction:    exactInstruction,
					SelectedAuthorities: append([]assemblyline.ContextCandidateAuthority(nil), group...),
				}
				if _, err := assemblyline.NewContextMinificationJob(input); err != nil {
					return "", totalCalls, fmt.Errorf("context minification group %d: %w", groupIndex+1, err)
				}
				decision, receipt, err := station.Minify(ctx, input)
				if err != nil {
					return "", totalCalls, fmt.Errorf("context minification group %d: %w", groupIndex+1, err)
				}
				if err := validateReceipt("context minification", receipt); err != nil {
					return "", totalCalls, err
				}
				totalCalls += receipt.Calls
				if err := decision.ValidateFor(input); err != nil {
					return "", totalCalls, err
				}
				content = decision.MinimalContext
			}
			hash := assemblyline.ExactObjectiveContextSHA(content)
			if _, duplicate := seenContent[hash]; duplicate {
				continue
			}
			seenContent[hash] = struct{}{}
			authority, err := assemblyline.NewContextCandidateAuthority(
				"context_reduction", fmt.Sprintf("CTX_%d", len(next)+1), content,
			)
			if err != nil {
				return "", totalCalls, fmt.Errorf("bind context reduction group %d: %w", groupIndex+1, err)
			}
			next = append(next, authority)
		}
		reduced := joinAuthorityContent(next)
		if len(reduced) <= assemblyline.MaxContextMinifiedBytes {
			return reduced, totalCalls, nil
		}
		if len(reduced) >= previousBytes {
			return "", totalCalls, fmt.Errorf(
				"context semantic reduction made no progress: before=%d after=%d target=%d",
				previousBytes, len(reduced), assemblyline.MaxContextMinifiedBytes,
			)
		}
		previousBytes = len(reduced)
		current = next
	}
	return "", totalCalls, fmt.Errorf("context semantic reduction produced no retained authority")
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
	for index, authority := range authorities {
		entryBytes := contextAuthorityProjectionBytes(authority)
		if entryBytes > maximumBytes {
			return nil, fmt.Errorf(
				"context authority %d requires %d bytes beyond the %d-byte per-call budget",
				index, entryBytes, maximumBytes,
			)
		}
		if len(current) == maximumCount || currentBytes+entryBytes > maximumBytes {
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
		len(authority.Content) + len(authority.ContentSHA256)
}

func joinAuthorityContent(authorities []assemblyline.ContextCandidateAuthority) string {
	contents := make([]string, len(authorities))
	for index, authority := range authorities {
		contents[index] = authority.Content
	}
	return strings.Join(contents, "\n\n")
}
