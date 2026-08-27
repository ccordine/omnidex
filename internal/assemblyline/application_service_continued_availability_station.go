package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

type applicationServiceContinuedAvailabilityPromptCandidate struct {
	CandidateID ApplicationServiceContinuedAvailabilityCandidateID `json:"candidate_id"`
	Meaning     string                                             `json:"meaning"`
}

func BuildApplicationServiceContinuedAvailabilityPrompt(
	input ApplicationServiceContinuedAvailabilityInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	candidates, err := json.Marshal(applicationServiceContinuedAvailabilityPromptCandidates())
	if err != nil {
		return "", fmt.Errorf("encode application service continued availability candidates: %w", err)
	}
	prompt := strings.Join([]string{
		"Determine exactly one semantic fact: whether the immutable request explicitly requires the completed software to remain available after build and verification.",
		"Select exactly one opaque candidate ID. A description of how software runs or what it produces does not by itself require continued availability.",
		"CODE_OWNED_CANDIDATES_JSON:\n" + string(candidates),
		"IMMUTABLE_USER_REQUEST:\n" + input.UserRequest,
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application service continued availability prompt exceeds %d bytes",
			maxPortablePayloadBytes,
		)
	}
	return prompt, nil
}

func applicationServiceContinuedAvailabilityPromptCandidates() []applicationServiceContinuedAvailabilityPromptCandidate {
	return []applicationServiceContinuedAvailabilityPromptCandidate{
		{
			CandidateID: ApplicationServiceAvailabilityNotRequiredCandidate,
			Meaning:     "The request does not explicitly require continued availability after build and verification.",
		},
		{
			CandidateID: ApplicationServiceAvailabilityRequiredCandidate,
			Meaning:     "The request explicitly requires continued availability after build and verification.",
		},
	}
}

func ApplicationServiceContinuedAvailabilityResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "candidate_id"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ApplicationServiceContinuedAvailabilitySchemaV1,
			},
			"candidate_id": map[string]any{
				"type": "string", "enum": []string{
					string(ApplicationServiceAvailabilityNotRequiredCandidate),
					string(ApplicationServiceAvailabilityRequiredCandidate),
				},
			},
		},
	)
}

func DecodeApplicationServiceContinuedAvailabilityResult(
	input ApplicationServiceContinuedAvailabilityInput,
	raw string,
) (ApplicationServiceContinuedAvailabilityResult, error) {
	var result ApplicationServiceContinuedAvailabilityResult
	if err := input.validate(); err != nil {
		return result, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return result, fmt.Errorf(
			"application service continued availability result exceeds %d bytes",
			maxPortableCandidateBytes,
		)
	}
	if err := decodePortablePayload([]byte(raw), &result); err != nil {
		return result, fmt.Errorf("decode application service continued availability result: %w", err)
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceContinuedAvailabilityResult{}, err
	}
	return result, nil
}
