package assemblyline

import "strings"

func BuildApplicationClassificationPrompt(input ApplicationClassificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the observable delivery surface of one software request.",
		"Choose browser_application only when the request requires an interactive browser page, and command_line_application only when it requires a terminal program.",
		"Choose unspecified only when the request does not constrain its observable delivery surface. Do not choose unsupported merely because the surface is omitted.",
		"Choose unsupported when the request explicitly requires an unregistered surface or incompatible multiple explicit surfaces.",
		"Output grammar: browser_application | command_line_application | unspecified | unsupported",
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}

func BuildArtifactHandlingPrompt(input ArtifactHandlingInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the user's explicit authority over FOCUSED_ARTIFACT.",
		"Choose preserve_unchanged when it must remain unchanged, must_exist when its existence is required, must_be_absent when its absence is explicitly required, possible_absence_candidate when it is one member of an unresolved explicit absence choice, or mentioned_only when it is referenced without another explicit state relation.",
		"Output grammar: preserve_unchanged | must_exist | must_be_absent | possible_absence_candidate | mentioned_only",
		"FOCUSED_ARTIFACT: " + input.Token,
		"CURRENT_REQUEST:\n" + input.UserRequest,
	}, "\n\n"), nil
}
