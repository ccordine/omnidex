package assemblyline

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type ApplicationIntentRepairInput struct {
	Authority ApplicationIntentInput          `json:"authority"`
	Finding   ApplicationIntentReviewDecision `json:"finding"`
}

type ApplicationIntentRepairDecision struct {
	Target      string
	Replacement string
}

func (input ApplicationIntentRepairInput) validate() error {
	if err := input.Authority.validate(); err != nil {
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
	projection := struct {
		Authority ApplicationIntentInput `json:"authority"`
		Target    string                 `json:"target"`
		Finding   string                 `json:"finding"`
	}{
		Authority: input.Authority,
		Target:    input.Finding.Target,
		Finding:   input.Finding.Finding,
	}
	authorityJSON, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application intent repair authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Replace exactly the named semantic leaf. The retained candidate and every accepted leaf remain code-owned and unavailable.",
		"Return only the one-field JSON replacement. Derive the minimum faithful replacement from the immutable user request, authoritative context facts, and reviewed finding. Do not alter another requirement, invent scope, plan work, choose files or paths, implement anything, or claim completion.",
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
	return objectSchema(
		[]string{input.Finding.Target},
		map[string]any{
			input.Finding.Target: map[string]any{
				"type": "string", "minLength": 1, "maxLength": maximum,
			},
		},
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
	replacement, ok := value.(string)
	if !ok {
		return zero, fmt.Errorf("application intent repair replacement must be a string")
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

func ApplyApplicationIntentRepair(
	authority ApplicationIntentInput,
	retained ApplicationIntentCandidate,
	finding ApplicationIntentReviewDecision,
	repair ApplicationIntentRepairDecision,
) (ApplicationIntentCandidate, error) {
	var zero ApplicationIntentCandidate
	reviewInput := ApplicationIntentReviewInput{Authority: authority, Candidate: retained}
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
		if corrected.ProductContext == repair.Replacement {
			return zero, fmt.Errorf("application intent repair is a no-op")
		}
		corrected.ProductContext = repair.Replacement
	} else {
		index, err := applicationIntentRequirementTargetIndex(repair.Target)
		if err != nil || index >= len(corrected.Requirements) {
			return zero, fmt.Errorf("application intent repair target %q is outside retained state", repair.Target)
		}
		if corrected.Requirements[index] == repair.Replacement {
			return zero, fmt.Errorf("application intent repair is a no-op")
		}
		corrected.Requirements[index] = repair.Replacement
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
