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
		"Return exactly the raw candidate ID and nothing else: no JSON, quotes, label, Markdown, or commentary.",
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

func DecodeApplicationServiceContinuedAvailabilityResult(
	input ApplicationServiceContinuedAvailabilityInput,
	raw string,
) (ApplicationServiceContinuedAvailabilityResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceContinuedAvailabilityResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf("application service continued availability", raw, 64, false)
	if err != nil {
		return ApplicationServiceContinuedAvailabilityResult{}, err
	}
	result := ApplicationServiceContinuedAvailabilityResult{
		Schema:      ApplicationServiceContinuedAvailabilitySchemaV1,
		CandidateID: ApplicationServiceContinuedAvailabilityCandidateID(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceContinuedAvailabilityResult{}, err
	}
	return result, nil
}
