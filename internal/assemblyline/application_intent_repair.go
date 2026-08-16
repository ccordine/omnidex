package assemblyline

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type ApplicationIntentRepairInput struct {
	Authority ApplicationIntentInput          `json:"authority"`
	Candidate ApplicationIntentCandidate      `json:"candidate"`
	Finding   ApplicationIntentReviewDecision `json:"finding"`
}

type ApplicationIntentRepairDecision struct {
	Target      string
	Replacement string
	Remove      bool
}

func (input ApplicationIntentRepairInput) validate() error {
	if err := input.Authority.validate(); err != nil {
		return err
	}
	if err := input.Candidate.Validate(); err != nil {
		return err
	}
	reviewInput := ApplicationIntentReviewInput{
		Authority: input.Authority, Candidate: input.Candidate, Target: input.Finding.Target,
	}
	if err := input.Finding.ValidateFor(reviewInput); err != nil {
		return err
	}
	if input.Finding.Schema != ApplicationIntentReviewSchemaV1 ||
		input.Finding.Outcome != ApplicationIntentReviewRepair {
		return fmt.Errorf("application intent repair requires one reviewed repair finding")
	}
	if !validApplicationIntentRepairTarget(input.Finding.Target) {
		return fmt.Errorf("application intent repair target %q is unsupported", input.Finding.Target)
	}
	return validateApplicationIntentText(
		"review finding", input.Finding.Finding, maxApplicationIntentFindingBytes,
	)
}

func BuildApplicationIntentRepairPrompt(input ApplicationIntentRepairInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	currentValue, err := applicationIntentTargetValue(input.Candidate, input.Finding.Target)
	if err != nil {
		return "", err
	}
	projection := struct {
		UserRequest   string `json:"immutable_user_request"`
		CurrentTarget string `json:"current_target"`
		CurrentValue  string `json:"current_value"`
		Problem       string `json:"problem"`
	}{
		UserRequest: input.Authority.UserRequest, CurrentTarget: input.Finding.Target,
		CurrentValue: currentValue, Problem: input.Finding.Finding,
	}
	authorityJSON, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application intent repair authority: %w", err)
	}
	request := "Return one replacement string for current_value that resolves problem under immutable_user_request."
	if applicationIntentRepairCanRemove(input) {
		request = "Return null when current_value is not required by immutable_user_request; otherwise return one replacement string that resolves problem."
	}
	prompt := strings.Join([]string{
		request,
		"The response is one JSON object containing only current_target.",
		"APPLICATION_INTENT_REPAIR_AUTHORITY_JSON:\n" + string(authorityJSON),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application intent repair prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func ApplicationIntentRepairResponseSchema(
	input ApplicationIntentRepairInput,
) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	maximum := maxRequirementQuoteBytes
	if input.Finding.Target == "product_context" {
		maximum = maxApplicationProductBytes
	}
	definition := any(map[string]any{
		"type": "string", "minLength": 1, "maxLength": maximum,
	})
	if applicationIntentRepairCanRemove(input) {
		definition = map[string]any{"oneOf": []any{
			map[string]any{"type": "string", "minLength": 1, "maxLength": maximum},
			map[string]any{"type": "null"},
		}}
	}
	return objectSchema(
		[]string{input.Finding.Target},
		map[string]any{input.Finding.Target: definition},
	), nil
}

func DecodeApplicationIntentRepairDecision(
	input ApplicationIntentRepairInput,
	raw string,
) (ApplicationIntentRepairDecision, error) {
	var zero ApplicationIntentRepairDecision
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("application intent repair exceeds %d bytes", maxPortableCandidateBytes)
	}
	patch, err := decodeJSONObject(raw, "application intent repair")
	if err != nil {
		return zero, err
	}
	if len(patch) != 1 {
		return zero, fmt.Errorf("application intent repair must contain exactly one leaf")
	}
	value, exists := patch[input.Finding.Target]
	if !exists {
		return zero, fmt.Errorf("application intent repair must replace %q", input.Finding.Target)
	}
	if value == nil {
		if !applicationIntentRepairCanRemove(input) {
			return zero, fmt.Errorf("application intent repair cannot remove the final required value")
		}
		return ApplicationIntentRepairDecision{Target: input.Finding.Target, Remove: true}, nil
	}
	replacement, ok := value.(string)
	if !ok {
		return zero, fmt.Errorf("application intent repair replacement must be a string or null")
	}
	maximum := maxRequirementQuoteBytes
	if input.Finding.Target == "product_context" {
		maximum = maxApplicationProductBytes
	}
	if err := validateApplicationIntentText("repair replacement", replacement, maximum); err != nil {
		return zero, err
	}
	return ApplicationIntentRepairDecision{
		Target: input.Finding.Target, Replacement: replacement,
	}, nil
}

func applicationIntentRepairCanRemove(input ApplicationIntentRepairInput) bool {
	return input.Finding.Target != "product_context" && len(input.Candidate.Requirements) > 1
}

func ApplyApplicationIntentRepair(
	authority ApplicationIntentInput,
	retained ApplicationIntentCandidate,
	finding ApplicationIntentReviewDecision,
	repair ApplicationIntentRepairDecision,
) (ApplicationIntentCandidate, error) {
	var zero ApplicationIntentCandidate
	reviewInput := ApplicationIntentReviewInput{
		Authority: authority, Candidate: retained, Target: finding.Target,
	}
	if err := finding.ValidateFor(reviewInput); err != nil {
		return zero, err
	}
	if finding.Outcome != ApplicationIntentReviewRepair {
		return zero, fmt.Errorf("application intent repair requires a repair finding")
	}
	if repair.Target != finding.Target {
		return zero, fmt.Errorf("application intent repair target does not match reviewed finding")
	}
	corrected := cloneApplicationIntentCandidate(retained)
	if repair.Target == "product_context" {
		if repair.Remove {
			return zero, fmt.Errorf("application intent product context cannot be removed")
		}
		if corrected.ProductContext == repair.Replacement {
			return zero, NewApplicationIntentRepairNoOpError(finding)
		}
		corrected.ProductContext = repair.Replacement
	} else {
		index, err := applicationIntentRequirementTargetIndex(repair.Target)
		if err != nil || index >= len(corrected.Requirements) {
			return zero, fmt.Errorf("application intent repair target %q is outside retained state", repair.Target)
		}
		if repair.Remove {
			corrected.Requirements = append(
				corrected.Requirements[:index], corrected.Requirements[index+1:]...,
			)
		} else if corrected.Requirements[index] == repair.Replacement {
			return zero, NewApplicationIntentRepairNoOpError(finding)
		} else {
			corrected.Requirements[index] = repair.Replacement
		}
	}
	if err := corrected.Validate(); err != nil {
		return zero, err
	}
	return corrected, nil
}

func validApplicationIntentRepairTarget(target string) bool {
	if target == "product_context" {
		return true
	}
	index, err := applicationIntentRequirementTargetIndex(target)
	return err == nil && index < maxRequirementCount
}

func applicationIntentRequirementTargetIndex(target string) (int, error) {
	const prefix = "requirements_"
	if !strings.HasPrefix(target, prefix) {
		return 0, fmt.Errorf("application intent requirement target %q is unsupported", target)
	}
	raw := strings.TrimPrefix(target, prefix)
	ordinal, err := strconv.Atoi(raw)
	if err != nil || ordinal < 1 || ordinal > maxRequirementCount || fmt.Sprintf("%03d", ordinal) != raw {
		return 0, fmt.Errorf("application intent requirement target %q is unsupported", target)
	}
	return ordinal - 1, nil
}
