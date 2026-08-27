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

func ApplicationServicePersistenceDestinationResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "candidate_id"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ApplicationServicePersistenceDestinationSchemaV1,
			},
			"candidate_id": map[string]any{
				"type": "string", "enum": []string{
					string(ApplicationServiceBuildEnvironmentDestinationCandidate),
					string(ApplicationServiceBuildEnvironmentNotEstablishedCandidate),
				},
			},
		},
	)
}

func DecodeApplicationServicePersistenceDestinationResult(
	input ApplicationServicePersistenceDestinationInput,
	raw string,
) (ApplicationServicePersistenceDestinationResult, error) {
	var result ApplicationServicePersistenceDestinationResult
	if err := input.validate(); err != nil {
		return result, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return result, fmt.Errorf(
			"application service persistence destination result exceeds %d bytes",
			maxPortableCandidateBytes,
		)
	}
	if err := decodePortablePayload([]byte(raw), &result); err != nil {
		return result, fmt.Errorf("decode application service persistence destination result: %w", err)
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServicePersistenceDestinationResult{}, err
	}
	return result, nil
}
