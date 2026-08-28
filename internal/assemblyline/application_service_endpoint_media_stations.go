package assemblyline

import (
	"fmt"
	"strconv"
	"strings"
)

func BuildApplicationServiceEndpointRequestMediaPrompt(
	input ApplicationServiceEndpointRequestMediaInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	candidates, err := ApplicationServiceEndpointRequestMediaCandidates(input)
	if err != nil {
		return "", err
	}
	values := make([]string, len(candidates))
	for index, candidate := range candidates {
		values[index] = string(candidate)
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine the one request media type required by this accepted local task and accepted HTTP method.",
		"Return exactly one raw request media value from this registered set: "+strings.Join(values, ", ")+".",
	)
}

func DecodeApplicationServiceEndpointRequestMediaResult(
	input ApplicationServiceEndpointRequestMediaInput,
	raw string,
) (ApplicationServiceEndpointRequestMediaResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointRequestMediaResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf("service endpoint request media", raw, 64, false)
	if err != nil {
		return ApplicationServiceEndpointRequestMediaResult{}, err
	}
	result := ApplicationServiceEndpointRequestMediaResult{
		Schema:       ApplicationServiceEndpointRequestMediaSchemaV1,
		RequestMedia: ApplicationServiceEndpointMedia(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceEndpointRequestMediaResult{}, err
	}
	return result, nil
}

func BuildApplicationServiceEndpointResponseMediaPrompt(
	input ApplicationServiceEndpointResponseMediaInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine the one response media type produced by this accepted local task.",
		"Return exactly one raw response media value from this registered set: "+
			strings.Join(applicationServiceResponseMediaValues(), ", ")+".",
	)
}

func DecodeApplicationServiceEndpointResponseMediaResult(
	input ApplicationServiceEndpointResponseMediaInput,
	raw string,
) (ApplicationServiceEndpointResponseMediaResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointResponseMediaResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf("service endpoint response media", raw, 64, false)
	if err != nil {
		return ApplicationServiceEndpointResponseMediaResult{}, err
	}
	result := ApplicationServiceEndpointResponseMediaResult{
		Schema:        ApplicationServiceEndpointResponseMediaSchemaV1,
		ResponseMedia: ApplicationServiceEndpointMedia(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceEndpointResponseMediaResult{}, err
	}
	return result, nil
}

func BuildApplicationServiceEndpointSuccessStatusPrompt(
	input ApplicationServiceEndpointSuccessStatusInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	candidates, err := ApplicationServiceEndpointSuccessStatusCandidates(input)
	if err != nil {
		return "", err
	}
	values := make([]string, len(candidates))
	for index, candidate := range candidates {
		values[index] = strconv.Itoa(candidate)
	}
	return buildApplicationServiceEndpointLeafPrompt(
		input,
		"Determine the one successful HTTP status compatible with this accepted local task, method, and media types.",
		"Return exactly one raw decimal success status from this registered set: "+strings.Join(values, ", ")+".",
	)
}

func DecodeApplicationServiceEndpointSuccessStatusResult(
	input ApplicationServiceEndpointSuccessStatusInput,
	raw string,
) (ApplicationServiceEndpointSuccessStatusResult, error) {
	if err := input.validate(); err != nil {
		return ApplicationServiceEndpointSuccessStatusResult{}, err
	}
	leaf, err := decodeRawSemanticLeaf("service endpoint success status", raw, 3, false)
	if err != nil {
		return ApplicationServiceEndpointSuccessStatusResult{}, err
	}
	status, err := strconv.Atoi(leaf)
	if err != nil || strconv.Itoa(status) != leaf {
		return ApplicationServiceEndpointSuccessStatusResult{}, fmt.Errorf(
			"service endpoint success status must be one canonical decimal integer",
		)
	}
	result := ApplicationServiceEndpointSuccessStatusResult{
		Schema:        ApplicationServiceEndpointSuccessStatusSchemaV1,
		SuccessStatus: status,
	}
	if err := result.ValidateFor(input); err != nil {
		return ApplicationServiceEndpointSuccessStatusResult{}, err
	}
	return result, nil
}
