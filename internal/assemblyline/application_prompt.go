package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func BuildApplicationClassificationPrompt(input ApplicationClassificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the observable delivery surface of one software request.",
		"Choose browser_application when the user expects an interactive browser page, command_line_application for a terminal program, service_application for a server/API without a requested browser interface, or unsupported when none applies. Do not identify the product, extract features, infer architecture, name files, choose a framework or language, or implement anything.",
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}

func BuildRepositoryRequirementInterpretationPrompt(
	input RepositoryRequirementInterpretationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	contextJSON, err := json.Marshal(input.Context)
	if err != nil {
		return "", fmt.Errorf("encode repository requirement context: %w", err)
	}
	return strings.Join([]string{
		"Translate one intact existing-repository request into the smallest complete ordered set of explicit change requirements.",
		"Each requirement is one concise semantic statement of requested behavior or constraint. Faithfully paraphrase the request without inventing implementation details or additional obligations.",
		"The context contains verified current source facts that may clarify the requested change.",
		"APPLICATION_CONTEXT_JSON:\n" + string(contextJSON),
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}

func BuildArtifactHandlingPrompt(input ArtifactHandlingInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the user's explicit authority over FOCUSED_ARTIFACT.",
		"Choose preserve_unchanged when it must not be modified, must_exist when its existence is required but its contents are not authorized to change, must_be_absent when the user explicitly requires this exact artifact itself to no longer exist, possible_absence_candidate only when this artifact is one member of an explicitly required absence choice whose exact member remains unresolved, or mentioned_only when it is merely referenced. Classify desired truth only: never choose a filesystem operation. Do not infer its identity, contents, path, or implementation role.",
		"FOCUSED_ARTIFACT: " + input.Token,
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}
