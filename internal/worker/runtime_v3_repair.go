package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/specialist"
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

func (r *nativeRuntimeV3) specialistRepairModels(skillID, original string, count int) []string {
	candidates := []string{strings.TrimSpace(original)}
	if skillID == "prompt_interpreter" {
		candidates = append(candidates,
			strings.TrimSpace(r.svc.models.Plan),
			strings.TrimSpace(r.svc.models.Specialist[specialist.RolePlannerSpecialist]),
			strings.TrimSpace(r.svc.models.Reasoning),
		)
	}
	candidates = append(candidates, strings.TrimSpace(r.svc.models.Analyze), strings.TrimSpace(r.svc.models.Fast))
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || containsStringExact(unique, candidate) {
			continue
		}
		unique = append(unique, candidate)
	}
	if len(unique) == 0 {
		unique = append(unique, strings.TrimSpace(original))
	}
	out := make([]string, count)
	for index := range out {
		out[index] = unique[min(index, len(unique)-1)]
	}
	return out
}

func buildV3SpecialistRepairPrompt(
	outputSchema json.RawMessage,
	invocation v3SpecialistInvocation,
	validationErr error,
	rejected string,
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
		"DIRECT CONTRACT CORRECTION. The previous JSON is rejected data, not an answer to repeat.",
		"VALIDATION_FAILURE:\n" + trimForBudget(validationErr.Error(), 5000),
		"Required action: change every field implicated by the failure now. Preserve unrelated valid fields. Repeating the rejected response is another failure.",
	}
	if action := v3SpecialistMechanicalRepairAction(validationErr); action != "" {
		sections = append(sections, "MECHANICAL_EDIT_REQUIRED:\n"+action)
	}
	sections = append(sections,
		"PREVIOUS_INVALID_RESPONSE:\n"+trimForBudget(rejected, 5000),
		"CURRENT_AUTHORITY:\n"+string(authorityJSON),
		"OUTPUT_SCHEMA:\n"+schema,
		"Return exactly one corrected raw JSON success envelope with contract_version 1.0 and role_id "+invocation.RoleID+". Omit error on success. Begin with { and end with }. Do not explain the correction.",
	)
	return strings.Join(sections, "\n\n"), nil
}

func v3SpecialistMechanicalRepairAction(validationErr error) string {
	if validationErr == nil {
		return ""
	}
	failure := strings.ToLower(validationErr.Error())
	actions := make([]string, 0, 2)
	if strings.Contains(failure, "global implementation constraint") {
		actions = append(actions, "Delete every exact array element identified by VALIDATION_FAILURE from the named acceptance_criteria or completion_criteria array. Do not paraphrase it or copy it to another criteria array. Keep or add the semantic restriction only in output.constraints.")
	}
	if strings.Contains(failure, "invented concrete path") {
		actions = append(actions, "Delete every criterion or description claim containing each invented path named by VALIDATION_FAILURE. Do not substitute a different concrete path unless CURRENT_AUTHORITY names it.")
	}
	return strings.Join(actions, "\n")
}
