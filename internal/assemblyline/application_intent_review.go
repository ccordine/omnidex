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
	Authority ApplicationIntentInput     `json:"authority"`
	Candidate ApplicationIntentCandidate `json:"candidate"`
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
	return input.Candidate.Validate()
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
		if !validApplicationIntentTarget(decision.Target, len(input.Candidate.Requirements)) {
			return fmt.Errorf("application intent review target %q is unsupported", decision.Target)
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
	authorityJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode application intent review authority: %w", err)
	}
	prompt := strings.Join([]string{
		"Review only whether candidate faithfully interprets the immutable user request under the authoritative context facts.",
		"Accept only when the product context and requirement statements collectively cover every explicit capability, behavior, user-visible element, and constraint without contradiction or invented scope. On failure, name exactly one defective leaf and one concise finding. Do not write replacement prose, design architecture, name files or paths, choose tools, plan work, implement anything, or claim completion.",
		"APPLICATION_INTENT_REVIEW_AUTHORITY_JSON:\n" + string(authorityJSON),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application intent review prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func ApplicationIntentReviewResponseSchema(input ApplicationIntentReviewInput) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	targets := applicationIntentTargets(len(input.Candidate.Requirements))
	properties := map[string]any{
		"schema":  map[string]any{"type": "string", "const": ApplicationIntentReviewSchemaV1},
		"outcome": enumSchema(ApplicationIntentReviewAccept, ApplicationIntentReviewRepair),
		"target":  enumSchema(targets...),
		"finding": map[string]any{
			"type": "string", "minLength": 1, "maxLength": maxApplicationIntentFindingBytes,
		},
	}
	return map[string]any{
		"type": "object", "properties": properties, "additionalProperties": false,
		"oneOf": []any{
			map[string]any{
				"required": []string{"schema", "outcome"},
				"properties": map[string]any{
					"schema":  map[string]any{"const": ApplicationIntentReviewSchemaV1},
					"outcome": map[string]any{"const": ApplicationIntentReviewAccept},
				},
			},
			map[string]any{
				"required": []string{"schema", "outcome", "target", "finding"},
				"properties": map[string]any{
					"schema":  map[string]any{"const": ApplicationIntentReviewSchemaV1},
					"outcome": map[string]any{"const": ApplicationIntentReviewRepair},
				},
			},
		},
	}, nil
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
	if err := decision.ValidateFor(input); err != nil {
		return zero, err
	}
	return decision, nil
}

func applicationIntentTargets(requirementCount int) []string {
	targets := make([]string, 0, requirementCount+1)
	targets = append(targets, "product_context")
	for index := 0; index < requirementCount; index++ {
		targets = append(targets, fmt.Sprintf("requirements_%03d", index+1))
	}
	return targets
}

func validApplicationIntentTarget(target string, requirementCount int) bool {
	for _, candidate := range applicationIntentTargets(requirementCount) {
		if target == candidate {
			return true
		}
	}
	return false
}
