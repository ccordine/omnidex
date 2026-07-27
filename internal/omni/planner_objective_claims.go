package omni

import (
	"fmt"
	"strings"
)

func mergePlannerObjectiveLedger(
	step int,
	current []StructuredObjective,
	claims []StructuredObjective,
	observations []StructuredCommandObservation,
	workingDirectory string,
	onEvent func(StructuredCommandEvent),
) []StructuredObjective {
	currentByID := make(map[string]StructuredObjective, len(current))
	for _, objective := range current {
		if normalized, ok := normalizeStructuredObjective(objective); ok {
			currentByID[normalized.ID] = normalized
		}
	}
	filtered := make([]StructuredObjective, 0, len(claims))
	for _, claim := range claims {
		normalized, ok := normalizeStructuredObjective(claim)
		if !ok {
			continue
		}
		if structuredObjectiveSatisfied(normalized) {
			candidate := normalized
			if existing, exists := currentByID[normalized.ID]; exists {
				if structuredObjectiveSatisfied(existing) {
					filtered = append(filtered, normalized)
					continue
				}
				candidate = mergeStructuredObjective(existing, normalized)
			}
			if !completionClaimHasDeterministicEvidence(candidate, observations, workingDirectory) {
				normalized.Status = "pending"
				normalized.Evidence = ""
				emitStructuredCommandEvent(onEvent, "planner_objective_claim_rejected_for_missing_evidence", "Planner objective claim remained pending because deterministic evidence is missing", map[string]string{
					"step":      fmt.Sprintf("%d", step),
					"objective": normalized.ID,
					"claim":     strings.TrimSpace(claim.Evidence),
				})
			}
		}
		filtered = append(filtered, normalized)
	}
	return mergeStructuredObjectiveLedger(current, filtered)
}
