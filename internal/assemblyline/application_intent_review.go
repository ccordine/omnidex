package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ApplicationIntentReviewSchemaV1  = "omnidex.application-intent-review.v1"
	maxApplicationIntentFindingBytes = 512
)

type ApplicationIntentReviewOutcome string

const (
	ApplicationIntentReviewAccept ApplicationIntentReviewOutcome = "accept"
	ApplicationIntentReviewRepair ApplicationIntentReviewOutcome = "repair"
)

type ApplicationIntentReviewInput struct {
	Authority      ApplicationIntentInput            `json:"authority"`
	Candidate      ApplicationIntentCandidate        `json:"candidate"`
	Target         string                            `json:"target"`
	PriorRejection *ApplicationIntentReviewRejection `json:"prior_rejection,omitempty"`
}

type ApplicationIntentReviewRejection struct {
	Target  string `json:"target"`
	Finding string `json:"finding"`
	Reason  string `json:"reason"`
}

type ApplicationIntentReviewDecision struct {
	Schema  string                         `json:"schema"`
	Outcome ApplicationIntentReviewOutcome `json:"outcome"`
	Target  string                         `json:"target,omitempty"`
	Finding string                         `json:"finding,omitempty"`
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
	if input.PriorRejection != nil {
		if input.PriorRejection.Target != input.Target ||
			input.PriorRejection.Reason != "repair_noop" {
			return fmt.Errorf("application intent review prior rejection is invalid")
		}
		if err := validateApplicationIntentText(
			"prior rejected finding", input.PriorRejection.Finding,
			maxApplicationIntentFindingBytes,
		); err != nil {
			return err
		}
	}
	return nil
}

func (decision ApplicationIntentReviewDecision) ValidateFor(
	input ApplicationIntentReviewInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ApplicationIntentReviewSchemaV1 {
		return fmt.Errorf("application intent review schema must be %q", ApplicationIntentReviewSchemaV1)
	}
	switch decision.Outcome {
	case ApplicationIntentReviewAccept:
		if decision.Target != "" || decision.Finding != "" {
			return fmt.Errorf("accepted application intent review must not include a target or finding")
		}
		return nil
	case ApplicationIntentReviewRepair:
		if decision.Target != input.Target {
			return fmt.Errorf("application intent review targeted %q instead of %q", decision.Target, input.Target)
		}
		if err := validateApplicationIntentText(
			"review finding", decision.Finding, maxApplicationIntentFindingBytes,
		); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("application intent review outcome %q is unsupported", decision.Outcome)
	}
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
		UserRequest    string                            `json:"immutable_user_request"`
		CurrentTarget  string                            `json:"current_target"`
		FieldContract  string                            `json:"field_contract"`
		CurrentValue   string                            `json:"current_value"`
		PriorRejection *ApplicationIntentReviewRejection `json:"prior_rejected_review,omitempty"`
	}{
		UserRequest: input.Authority.UserRequest, CurrentTarget: input.Target,
		FieldContract: applicationIntentTargetContract(input.Target), CurrentValue: currentValue,
		PriorRejection: input.PriorRejection,
	}
	authorityJSON, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application intent review authority: %w", err)
	}
	parts := []string{
		"Determine whether current_value satisfies field_contract under immutable_user_request.",
		"A repair finding must name a conflict, unsupported addition, or semantic error; repeating current_value is not a finding.",
		"Return {\"schema\":\"omnidex.application-intent-review.v1\",\"outcome\":\"accept\",\"finding\":\"\"} or {\"schema\":\"omnidex.application-intent-review.v1\",\"outcome\":\"repair\",\"finding\":<the exact semantic problem>}.",
	}
	if input.PriorRejection != nil {
		parts = append(parts,
			"The prior rejected finding produced a byte-identical replacement for current_value. Re-evaluate current_value; accept it or return a different exact semantic problem.",
		)
	}
	parts = append(parts, "APPLICATION_INTENT_REVIEW_INPUT_JSON:\n"+string(authorityJSON))
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
	properties := map[string]any{
		"schema":  map[string]any{"type": "string", "const": ApplicationIntentReviewSchemaV1},
		"outcome": enumSchema(ApplicationIntentReviewAccept, ApplicationIntentReviewRepair),
		"finding": map[string]any{
			"type": "string", "minLength": 0, "maxLength": maxApplicationIntentFindingBytes,
		},
	}
	return objectSchema([]string{"schema", "outcome", "finding"}, properties), nil
}

func DecodeApplicationIntentReviewDecision(
	input ApplicationIntentReviewInput,
	raw string,
) (ApplicationIntentReviewDecision, error) {
	var zero ApplicationIntentReviewDecision
	if err := input.validate(); err != nil {
		return zero, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return zero, fmt.Errorf("application intent review exceeds %d bytes", maxPortableCandidateBytes)
	}
	var decision ApplicationIntentReviewDecision
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return zero, fmt.Errorf("decode application intent review: %w", err)
	}
	if decision.Outcome == ApplicationIntentReviewRepair {
		decision.Target = input.Target
	} else if decision.Outcome == ApplicationIntentReviewAccept {
		decision.Finding = ""
	}
	if err := decision.ValidateFor(input); err != nil {
		return zero, err
	}
	return decision, nil
}

func applicationIntentTargetValue(
	candidate ApplicationIntentCandidate,
	target string,
) (string, error) {
	if target == "product_context" {
		return candidate.ProductContext, nil
	}
	index, err := applicationIntentRequirementTargetIndex(target)
	if err != nil || index >= len(candidate.Requirements) {
		return "", fmt.Errorf("application intent target %q is outside retained state", target)
	}
	return candidate.Requirements[index], nil
}

// ApplicationIntentReviewTargetValue returns the exact retained leaf selected
// by code for one review call.
func ApplicationIntentReviewTargetValue(
	candidate ApplicationIntentCandidate,
	target string,
) (string, error) {
	return applicationIntentTargetValue(candidate, target)
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

// ApplicationIntentReviewTargets returns the code-owned semantic-leaf order
// for one already validated retained intent candidate.
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
