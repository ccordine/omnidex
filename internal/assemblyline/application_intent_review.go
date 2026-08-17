package assemblyline

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const ApplicationIntentReviewSchemaV1 = "omnidex.application-intent-review.v1"

type ApplicationIntentReviewDecision string

const (
	ApplicationIntentReviewAccept  ApplicationIntentReviewDecision = "accept"
	ApplicationIntentReviewRemove  ApplicationIntentReviewDecision = "remove"
	ApplicationIntentReviewReplace ApplicationIntentReviewDecision = "replace"
)

// ApplicationIntentReview is a model-proposed semantic leaf. The fields that
// bind it to retained authority are code-private and can only be established
// while decoding a response for one exact retained target.
type ApplicationIntentReview struct {
	Decision         ApplicationIntentReviewDecision `json:"decision"`
	Target           string                          `json:"target,omitempty"`
	CurrentValue     string                          `json:"current_value,omitempty"`
	ReplacementValue string                          `json:"replacement_value,omitempty"`

	requestSHA256 string
	valueSHA256   string
}

type ApplicationIntentReviewInput struct {
	Authority ApplicationIntentInput     `json:"authority"`
	Candidate ApplicationIntentCandidate `json:"candidate"`
	Target    string                     `json:"target"`
}

type applicationIntentReviewWire struct {
	Schema           *string                          `json:"schema"`
	Decision         *ApplicationIntentReviewDecision `json:"decision"`
	ReplacementValue *string                          `json:"replacement_value"`
}

func (input ApplicationIntentReviewInput) validate() error {
	if err := input.Authority.validate(); err != nil {
		return err
	}
	if err := input.Candidate.Validate(); err != nil {
		return err
	}
	if !validApplicationIntentTarget(input.Target, len(input.Candidate.Requirements)) {
		return fmt.Errorf("application intent review target %q is unsupported", input.Target)
	}
	return nil
}

func BuildApplicationIntentReviewPrompt(input ApplicationIntentReviewInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	currentValue, err := applicationIntentTargetValue(input.Candidate, input.Target)
	if err != nil {
		return "", err
	}
	projection := struct {
		UserRequest   string `json:"immutable_user_request"`
		CurrentTarget string `json:"current_target"`
		FieldContract string `json:"field_contract"`
		CurrentValue  string `json:"current_value"`
	}{
		UserRequest: input.Authority.UserRequest, CurrentTarget: input.Target,
		FieldContract: applicationIntentTargetContract(input.Target), CurrentValue: currentValue,
	}
	authorityJSON, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application intent review authority: %w", err)
	}
	parts := []string{
		"Determine whether current_value satisfies field_contract under immutable_user_request.",
		"Return exactly one decision about this one current value.",
		"Accept: {\"schema\":\"omnidex.application-intent-review.v1\",\"decision\":\"accept\",\"replacement_value\":\"\"}.",
		"Remove: only when current_target is a wholly irrelevant requirement and another requirement remains; return replacement_value as an empty string.",
		"Replace: return a complete corrected current_value in replacement_value. Return the value itself, not an explanation, instruction, plan, patch, or JSON fragment.",
		"APPLICATION_INTENT_REVIEW_INPUT_JSON:\n" + string(authorityJSON),
	}
	prompt := strings.Join(parts, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application intent review prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func ApplicationIntentReviewResponseSchema(input ApplicationIntentReviewInput) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	maximum := maxRequirementQuoteBytes
	if input.Target == "product_context" {
		maximum = maxApplicationProductBytes
	}
	return objectSchema(
		[]string{"schema", "decision", "replacement_value"},
		map[string]any{
			"schema":            map[string]any{"type": "string", "const": ApplicationIntentReviewSchemaV1},
			"decision":          enumSchema(ApplicationIntentReviewAccept, ApplicationIntentReviewRemove, ApplicationIntentReviewReplace),
			"replacement_value": map[string]any{"type": "string", "minLength": 0, "maxLength": maximum},
		},
	), nil
}

func DecodeApplicationIntentReview(
	input ApplicationIntentReviewInput,
	raw string,
) (ApplicationIntentReview, error) {
	var zero ApplicationIntentReview
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("application intent review exceeds %d bytes", maxPortableCandidateBytes)
	}
	var wire applicationIntentReviewWire
	if err := decodePortablePayload([]byte(raw), &wire); err != nil {
		return zero, fmt.Errorf("decode application intent review: %w", err)
	}
	if wire.Schema == nil || *wire.Schema != ApplicationIntentReviewSchemaV1 ||
		wire.Decision == nil || wire.ReplacementValue == nil {
		return zero, fmt.Errorf("application intent review requires schema, decision, and replacement_value")
	}
	current, err := applicationIntentTargetValue(input.Candidate, input.Target)
	if err != nil {
		return zero, err
	}
	review := ApplicationIntentReview{
		Decision: *wire.Decision, Target: input.Target, CurrentValue: current,
		requestSHA256: input.Authority.Context.RequestSHA256, valueSHA256: ExactObjectiveContextSHA(current),
	}
	switch review.Decision {
	case ApplicationIntentReviewAccept:
		if *wire.ReplacementValue != "" {
			return zero, fmt.Errorf("accepted application intent review must not provide a replacement")
		}
		return review, nil
	case ApplicationIntentReviewRemove:
		if *wire.ReplacementValue != "" {
			return zero, fmt.Errorf("removed application intent review must not provide a replacement")
		}
		if !applicationIntentReviewCanRemove(input.Candidate, input.Target) {
			return zero, fmt.Errorf("application intent review cannot remove the final required value")
		}
		return review, nil
	case ApplicationIntentReviewReplace:
		maximum := maxRequirementQuoteBytes
		if input.Target == "product_context" {
			maximum = maxApplicationProductBytes
		}
		if err := validateApplicationIntentText("review replacement", *wire.ReplacementValue, maximum); err != nil {
			return zero, err
		}
		if *wire.ReplacementValue == current {
			return zero, fmt.Errorf("application intent review replacement is byte-identical to current value")
		}
		review.ReplacementValue = *wire.ReplacementValue
		return review, nil
	default:
		return zero, fmt.Errorf("application intent review decision %q is unsupported", review.Decision)
	}
}

func ApplicationIntentReviewTargetValue(candidate ApplicationIntentCandidate, target string) (string, error) {
	return applicationIntentTargetValue(candidate, target)
}

func applicationIntentTargetValue(candidate ApplicationIntentCandidate, target string) (string, error) {
	if target == "product_context" {
		return candidate.ProductContext, nil
	}
	index, err := applicationIntentRequirementTargetIndex(target)
	if err != nil || index >= len(candidate.Requirements) {
		return "", fmt.Errorf("application intent target %q is outside retained state", target)
	}
	return candidate.Requirements[index], nil
}

func applicationIntentTargetContract(target string) string {
	if target == "product_context" {
		return "describe the software product requested by the user"
	}
	return "state one product obligation explicitly required by the user request without invented precision or scope; sibling obligations do not need to be repeated"
}

func applicationIntentTargets(requirementCount int) []string {
	targets := make([]string, 0, requirementCount+1)
	targets = append(targets, "product_context")
	for index := 0; index < requirementCount; index++ {
		targets = append(targets, fmt.Sprintf("requirements_%03d", index+1))
	}
	return targets
}

func ApplicationIntentReviewTargets(candidate ApplicationIntentCandidate) []string {
	return applicationIntentTargets(len(candidate.Requirements))
}

func validApplicationIntentTarget(target string, requirementCount int) bool {
	for _, candidate := range applicationIntentTargets(requirementCount) {
		if target == candidate {
			return true
		}
	}
	return false
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
