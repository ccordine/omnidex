package assemblyline

import (
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

func BuildApplicationRequirementInterpretationPrompt(
	input ApplicationRequirementInterpretationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Extract the explicit product and requested features from one intact software request.",
		"Return exactly one product item whose source_quote is the shortest exact contiguous quote naming what is being built. Return between one and ten feature items covering every explicitly requested capability, behavior, user-visible element, or constraint. Each source_quote must be the shortest exact contiguous source text that preserves its meaningful action and modifiers.",
		"Never paraphrase, infer an unstated requirement, merge unrelated requirements, classify the delivery surface, design architecture, create tasks, choose files, or implement anything.",
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}

func BuildRepositoryRequirementInterpretationPrompt(
	input RepositoryRequirementInterpretationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Extract the explicit requested changes from one intact existing-repository request.",
		"Return between one and ten feature_quotes. Each must be the shortest exact contiguous source text that preserves one requested capability, behavior, user-visible change, or constraint and its meaningful modifiers.",
		"Never paraphrase, infer an unstated change, identify a product, merge unrelated changes, choose artifacts or paths, design architecture, create tasks, or implement anything.",
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
