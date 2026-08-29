package webresearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/websearch"
)

const (
	projectionTruncationMarker = "\n...[projection truncated]"
	relevanceTruncationMarker  = "...[truncated]"
)

func (machine *Machine) selectAndProject(
	ctx context.Context,
	evidence []Evidence,
	result *Result,
) ([]ProjectedEvidence, bool, error) {
	if err := validateEvidence(evidence); err != nil {
		return nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		machine.objective.KnownArtifactPaths,
	)
	if err != nil {
		return nil, false, fmt.Errorf("web relevance artifact provenance: %w", err)
	}
	candidates, err := buildRelevanceCandidates(
		evidence, machine.config.CandidateSummaryBytes, provenance,
	)
	if err != nil {
		return nil, false, err
	}
	call := RelevanceCall{
		Question:      machine.objective.Question,
		Context:       assemblyline.CloneObjectiveContext(machine.objective.Context),
		Candidates:    candidates,
		MaxSelections: min(machine.config.MaxRelevantCandidates, len(evidence)),
	}
	decision, err := machine.relevance.Select(ctx, cloneRelevanceCall(call))
	result.RelevanceCalls++
	if err != nil {
		return nil, false, fmt.Errorf("relevance station: %w", err)
	}
	if decision.SemanticCalls < 1 {
		return nil, false, fmt.Errorf("%w: relevance reported no semantic calls", ErrInvalidRelevance)
	}
	result.SemanticCalls += decision.SemanticCalls
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	selected, err := validateRelevanceDecision(decision, evidence, machine.config.MaxRelevantCandidates)
	if err != nil {
		return nil, false, err
	}
	result.Steps = append(result.Steps, StepRelevanceResolved)
	if decision.Outcome == RelevanceNone {
		return nil, false, nil
	}
	projected, err := buildProjection(
		selected, machine.config.MaxProjectionBytes, provenance,
	)
	return projected, err == nil, err
}

func fullProjectionBytes(evidence []Evidence) int {
	total := 0
	for _, item := range evidence {
		total += len(item.ID) + len(item.CandidateID) + len(item.Title) + len(item.Snippet) + len(item.Content)
	}
	return total
}

func buildRelevanceCandidates(
	evidence []Evidence,
	summaryBytes int,
	provenance modelcontext.ArtifactIdentityProvenance,
) ([]RelevanceCandidate, error) {
	result := make([]RelevanceCandidate, len(evidence))
	for index, item := range evidence {
		title, err := projectWebEvidenceModelText("web relevance title", item.Title, provenance)
		if err != nil {
			return nil, err
		}
		snippet, err := projectWebEvidenceModelText("web relevance snippet", item.Snippet, provenance)
		if err != nil {
			return nil, err
		}
		excerpt, err := projectWebEvidenceModelText("web relevance excerpt", item.Content, provenance)
		if err != nil {
			return nil, err
		}
		perField := summaryBytes / 3
		result[index] = RelevanceCandidate{
			CandidateID: item.CandidateID,
			Title:       truncateRelevanceField(title, perField),
			Snippet:     truncateRelevanceField(snippet, perField),
			Excerpt:     truncateRelevanceField(excerpt, summaryBytes-2*perField),
		}
	}
	return result, nil
}

func truncateRelevanceField(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= len(relevanceTruncationMarker) {
		return truncateBytes(relevanceTruncationMarker, limit)
	}
	return truncateBytes(value, limit-len(relevanceTruncationMarker)) + relevanceTruncationMarker
}

func validateRelevanceDecision(decision RelevanceDecision, evidence []Evidence, limit int) ([]Evidence, error) {
	if decision.CandidateIDs == nil {
		return nil, fmt.Errorf("%w: candidate IDs must be an explicit array", ErrInvalidRelevance)
	}
	if decision.Outcome == RelevanceNone {
		if len(decision.CandidateIDs) != 0 {
			return nil, fmt.Errorf("%w: NONE selected candidate IDs", ErrInvalidRelevance)
		}
		return nil, nil
	}
	if decision.Outcome != RelevanceSelected {
		return nil, fmt.Errorf("%w: outcome %q is unsupported", ErrInvalidRelevance, decision.Outcome)
	}
	if len(decision.CandidateIDs) < 1 || len(decision.CandidateIDs) > limit {
		return nil, fmt.Errorf("%w: selected outcome requires 1..%d candidate IDs", ErrInvalidRelevance, limit)
	}
	requested := make(map[websearch.CandidateID]struct{}, len(decision.CandidateIDs))
	known := make(map[websearch.CandidateID]Evidence, len(evidence))
	for _, item := range evidence {
		known[item.CandidateID] = item
	}
	for _, id := range decision.CandidateIDs {
		if _, duplicate := requested[id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate candidate ID %q", ErrInvalidRelevance, id)
		}
		if _, exists := known[id]; !exists {
			return nil, fmt.Errorf("%w: unknown candidate ID %q", ErrInvalidRelevance, id)
		}
		requested[id] = struct{}{}
	}
	// Selection is a set. Code preserves authoritative evidence order rather
	// than granting output ordering authority to the station.
	selected := make([]Evidence, 0, len(requested))
	for _, item := range evidence {
		if _, keep := requested[item.CandidateID]; keep {
			selected = append(selected, item)
		}
	}
	return selected, nil
}

func buildProjection(
	evidence []Evidence,
	budget int,
	provenance modelcontext.ArtifactIdentityProvenance,
) ([]ProjectedEvidence, error) {
	if len(evidence) == 0 || budget < len(evidence) {
		return nil, fmt.Errorf("%w: evidence cannot fit projection bound", ErrInvalidConfiguration)
	}
	modelEvidence := make([]Evidence, len(evidence))
	for index, item := range evidence {
		modelEvidence[index] = item
		var err error
		modelEvidence[index].Title, err = projectWebEvidenceModelText(
			"web synthesis title", item.Title, provenance,
		)
		if err != nil {
			return nil, err
		}
		modelEvidence[index].Snippet, err = projectWebEvidenceModelText(
			"web synthesis snippet", item.Snippet, provenance,
		)
		if err != nil {
			return nil, err
		}
		modelEvidence[index].Content, err = projectWebEvidenceModelText(
			"web synthesis content", item.Content, provenance,
		)
		if err != nil {
			return nil, err
		}
	}
	if fullProjectionBytes(modelEvidence) <= budget {
		result := make([]ProjectedEvidence, len(evidence))
		for index, item := range modelEvidence {
			result[index] = ProjectedEvidence{
				EvidenceID: item.ID, CandidateID: item.CandidateID,
				Title: item.Title, Snippet: item.Snippet, Content: item.Content,
			}
		}
		return result, nil
	}
	share := budget / len(evidence)
	result := make([]ProjectedEvidence, len(evidence))
	used := 0
	for index, item := range modelEvidence {
		entryBudget := share
		if index == len(evidence)-1 {
			entryBudget = budget - used
		}
		identityBytes := len(item.ID) + len(item.CandidateID)
		if entryBudget <= identityBytes+2 {
			return nil, fmt.Errorf("%w: projection bound cannot carry evidence identities", ErrInvalidConfiguration)
		}
		if entryBudget <= identityBytes+len(projectionTruncationMarker) {
			return nil, fmt.Errorf("%w: projection bound cannot carry explicit truncation authority", ErrInvalidConfiguration)
		}
		remaining := entryBudget - identityBytes - len(projectionTruncationMarker)
		titleBudget := remaining / 8
		snippetBudget := remaining / 4
		title := truncateBytes(item.Title, titleBudget)
		snippet := truncateBytes(item.Snippet, snippetBudget)
		contentBudget := remaining - len(title) - len(snippet)
		content := truncateBytes(item.Content, contentBudget)
		if content == "" {
			return nil, fmt.Errorf("%w: projection omitted evidence content", ErrInvalidConfiguration)
		}
		truncated := title != item.Title || snippet != item.Snippet || content != item.Content
		if !truncated {
			return nil, fmt.Errorf("%w: bounded projection accounting is inconsistent", ErrInvalidConfiguration)
		}
		content += projectionTruncationMarker
		result[index] = ProjectedEvidence{
			EvidenceID: item.ID, CandidateID: item.CandidateID,
			Title: title, Snippet: snippet, Content: content, Truncated: true,
		}
		used += projectionBytes(result[index])
	}
	if used > budget {
		return nil, fmt.Errorf("%w: projection used %d bytes over bound %d", ErrInvalidConfiguration, used, budget)
	}
	return result, nil
}

func projectWebEvidenceModelText(
	label string,
	value string,
	provenance modelcontext.ArtifactIdentityProvenance,
) (string, error) {
	matches := modelcontext.PathIdentities(value, provenance)
	if len(matches) == 0 {
		if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
			label, provenance, value,
		); err != nil {
			return "", err
		}
		return value, nil
	}
	var projected strings.Builder
	previous := 0
	for _, match := range matches {
		projected.WriteString(value[previous:match.Start])
		projected.WriteString("ARTIFACT_REF")
		previous = match.End
	}
	projected.WriteString(value[previous:])
	result := projected.String()
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		label, provenance, result,
	); err != nil {
		return "", err
	}
	return result, nil
}

func applyProjectionTruncation(
	evidence []Evidence,
	projected []ProjectedEvidence,
) ([]Evidence, error) {
	truncatedByID := make(map[EvidenceID]bool, len(projected))
	for _, item := range projected {
		if _, duplicate := truncatedByID[item.EvidenceID]; duplicate {
			return nil, fmt.Errorf("%w: projected evidence ID %q is duplicated", ErrInvalidAcquisition, item.EvidenceID)
		}
		truncatedByID[item.EvidenceID] = item.Truncated
	}
	result := cloneEvidence(evidence)
	for index := range result {
		if truncated, selected := truncatedByID[result[index].ID]; selected && truncated {
			result[index].Truncated = true
		}
	}
	return result, nil
}

func projectionBytes(value ProjectedEvidence) int {
	return len(value.EvidenceID) + len(value.CandidateID) + len(value.Title) + len(value.Snippet) + len(value.Content)
}
