package worker

import (
	"encoding/json"
	"fmt"
	"strings"
)

const maxV3SpecialistRepairPasses = 3

func v3SpecialistRepairLimit(policy string) int {
	switch strings.TrimSpace(policy) {
	case "one_repair_pass":
		return 1
	case "bounded_repair_passes":
		return maxV3SpecialistRepairPasses
	default:
		return 0
	}
}

func buildV3SpecialistRepairPrompt(
	outputSchema json.RawMessage,
	invocation v3SpecialistInvocation,
	validationErr error,
) (string, error) {
	if validationErr == nil {
		return "", fmt.Errorf("specialist repair requires a validation failure")
	}
	authorityJSON, err := json.MarshalIndent(struct {
		RoleID          string         `json:"role_id"`
		ObjectiveID     string         `json:"objective_id"`
		Objective       string         `json:"objective"`
		SuccessCriteria []string       `json:"success_criteria"`
		Payload         map[string]any `json:"payload"`
	}{invocation.RoleID, invocation.ObjectiveID, invocation.Objective, invocation.SuccessCriteria, invocation.Payload}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal specialist repair authority: %w", err)
	}
	schema := strings.TrimSpace(string(outputSchema))
	if schema == "" {
		return "", fmt.Errorf("specialist %s has no output schema", invocation.RoleID)
	}
	sections := []string{
		"DIRECT CONTRACT CORRECTION.",
		"VALIDATION_FAILURE:\n" + trimForBudget(validationErr.Error(), 5000),
		"Required action: correct every field implicated by the failure now. Preserve every unrelated requirement from CURRENT_AUTHORITY.",
	}
	sections = append(sections,
		"CURRENT_AUTHORITY:\n"+string(authorityJSON),
		"OUTPUT_SCHEMA:\n"+schema,
		"Return exactly one corrected raw JSON success envelope with contract_version 1.0 and role_id "+invocation.RoleID+". Omit error on success. Begin with { and end with }. Do not explain the correction.",
	)
	return strings.Join(sections, "\n\n"), nil
}
