package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

type applicationServicePersistenceDestinationPromptCandidate struct {
	CandidateID ApplicationServicePersistenceDestinationCandidateID `json:"candidate_id"`
	Meaning     string                                              `json:"meaning"`
}

func BuildApplicationServicePersistenceDestinationPrompt(
	input ApplicationServicePersistenceDestinationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	candidates, err := json.Marshal(applicationServicePersistenceDestinationPromptCandidates())
	if err != nil {
		return "", fmt.Errorf("encode application service persistence destination candidates: %w", err)
	}
	prompt := strings.Join([]string{
		"Determine exactly one semantic fact: whether the immutable request explicitly identifies the environment where the software is built as the continued-availability destination.",
		"Select exactly one opaque candidate ID. A destination reference whose identity could merely be the build environment does not explicitly establish identity.",
		"Return exactly the raw candidate ID and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"CODE_OWNED_CANDIDATES_JSON:\n" + string(candidates),
		"IMMUTABLE_USER_REQUEST:\n" + input.UserRequest,
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application service persistence destination prompt exceeds %d bytes",
			maxPortablePayloadBytes,
		)
	}
	return prompt, nil
}

func applicationServicePersistenceDestinationPromptCandidates() []applicationServicePersistenceDestinationPromptCandidate {
	return []applicationServicePersistenceDestinationPromptCandidate{
		{
			CandidateID: ApplicationServiceBuildEnvironmentDestinationCandidate,
			Meaning:     "The request explicitly identifies the environment where the software is built as the continued-availability destination.",
		},
		{
			CandidateID: ApplicationServiceBuildEnvironmentNotEstablishedCandidate,
			Meaning:     "The request does not explicitly identify the environment where the software is built as the continued-availability destination.",
		},
	}
}

func DecodeApplicationServicePersistenceDestinationResult(
	input ApplicationServicePersistenceDestinationInput,
	raw string,
) (ApplicationServicePersistenceDestinationResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServicePersistenceDestinationResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf("application service persistence destination", raw, 64, false)
	if err != nil {
		return ApplicationServicePersistenceDestinationResult{}, err
	}
	result := ApplicationServicePersistenceDestinationResult{
		Schema:      ApplicationServicePersistenceDestinationSchemaV1,
		CandidateID: ApplicationServicePersistenceDestinationCandidateID(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServicePersistenceDestinationResult{}, err
	}
	return result, nil
}
