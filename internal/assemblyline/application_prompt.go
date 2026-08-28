package assemblyline

import "strings"

func BuildApplicationClassificationPrompt(input ApplicationClassificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the observable delivery surface of one software request.",
		"Choose browser_application when the user expects an interactive browser page, command_line_application for a terminal program, service_application for a server/API without a requested browser interface, or unsupported when none applies. Do not identify the product, extract features, infer architecture, name files, choose a framework or language, or implement anything.",
		"Return exactly one raw registered surface value with no JSON, quotes, label, Markdown, or commentary.",
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
		"Return exactly one raw registered handling value with no JSON, quotes, label, Markdown, or commentary.",
		"FOCUSED_ARTIFACT: " + input.Token,
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}
