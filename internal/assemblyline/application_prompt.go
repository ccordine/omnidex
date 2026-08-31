package assemblyline

import "strings"

func BuildApplicationClassificationPrompt(input ApplicationClassificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Classify only the observable delivery surface of one software request.",
		"Choose browser_application for an interactive browser page, command_line_application for a terminal program, or unsupported when neither applies.",
		"Output grammar: browser_application | command_line_application | unsupported",
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
