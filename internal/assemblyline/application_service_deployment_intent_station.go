package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

type applicationServiceDeploymentPromptCandidate struct {
	CandidateID ApplicationServiceDeploymentCandidateID `json:"candidate_id"`
	Meaning     string                                  `json:"meaning"`
}

func BuildApplicationServiceDeploymentIntentPrompt(
	input ApplicationServiceDeploymentIntentInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	candidates, err := json.Marshal(applicationServiceDeploymentPromptCandidates())
	if err != nil {
		return "", fmt.Errorf("encode application service deployment candidates: %w", err)
	}
	prompt := strings.Join([]string{
		"Determine exactly one semantic fact: which post-verification runtime disposition is explicitly required by the immutable request.",
		"Select exactly one opaque candidate ID. Do not infer a persistence request when none is stated.",
		"CODE_OWNED_CANDIDATES_JSON:\n" + string(candidates),
		"IMMUTABLE_USER_REQUEST:\n" + input.UserRequest,
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application service deployment intent prompt exceeds %d bytes",
			maxPortablePayloadBytes,
		)
	}
	return prompt, nil
}

func applicationServiceDeploymentPromptCandidates() []applicationServiceDeploymentPromptCandidate {
	return []applicationServiceDeploymentPromptCandidate{
		{
			CandidateID: ApplicationServiceDeploymentNoPersistenceCandidate,
			Meaning:     "The request does not require the completed software to remain running after verification.",
		},
		{
			CandidateID: ApplicationServiceDeploymentCurrentHostCandidate,
			Meaning:     "The request explicitly requires the completed software to remain running in the same environment where it is built.",
		},
		{
			CandidateID: ApplicationServiceDeploymentOtherTargetCandidate,
			Meaning:     "The request requires continued availability at a different destination, or does not identify enough destination authority to use the build environment.",
		},
	}
}

func ApplicationServiceDeploymentIntentResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "candidate_id"},
		map[string]any{
			"schema": map[string]any{
				"type": "string", "const": ApplicationServiceDeploymentIntentSchemaV1,
			},
			"candidate_id": map[string]any{
				"type": "string", "enum": []string{
					string(ApplicationServiceDeploymentNoPersistenceCandidate),
					string(ApplicationServiceDeploymentCurrentHostCandidate),
					string(ApplicationServiceDeploymentOtherTargetCandidate),
				},
			},
		},
	)
}

func DecodeApplicationServiceDeploymentIntentResult(
	input ApplicationServiceDeploymentIntentInput,
	raw string,
) (ApplicationServiceDeploymentIntentResult, error) {
	var result ApplicationServiceDeploymentIntentResult
	if err := input.validate(); err != nil {
		return result, err
	}
	if len(raw) > maxPortableCandidateBytes {
		return result, fmt.Errorf(
			"application service deployment intent result exceeds %d bytes",
			maxPortableCandidateBytes,
		)
	}
	if err := decodePortablePayload([]byte(raw), &result); err != nil {
		return result, fmt.Errorf("decode application service deployment intent result: %w", err)
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceDeploymentIntentResult{}, err
	}
	return result, nil
}
