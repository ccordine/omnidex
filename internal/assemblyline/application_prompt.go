package assemblyline

func BuildApplicationClassificationPrompt(input ApplicationClassificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := applicationClassificationOpaqueChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"What kind of interface does the request describe?",
		[]string{"Software request:\n" + input.UserRequest},
		choices,
	)
}

func BuildArtifactHandlingPrompt(input ArtifactHandlingInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	choices, err := artifactHandlingOpaqueChoices()
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Which description matches only the user's explicit authority over the focused artifact?",
		[]string{
			"Focused artifact: " + input.Token,
			"Software request:\n" + input.UserRequest,
		},
		choices,
	)
}

func applicationClassificationOpaqueChoices() ([]OpaqueModelChoice, error) {
	definitions := []struct {
		description string
		value       string
	}{
		{"A browser app or web page.", string(ApplicationSurfaceBrowser)},
		{"A command-line program or terminal interface.", string(ApplicationSurfaceCommandLine)},
		{"The request does not say what interface to use.", string(ApplicationSurfaceUnspecified)},
		{"Another interface, such as a native mobile or graphical desktop app, or a combination of interfaces.", string(ApplicationSurfaceUnsupported)},
	}
	choices := make([]OpaqueModelChoice, 0, len(definitions))
	for _, definition := range definitions {
		choice, err := NewOpaqueModelChoice(definition.description, definition.value)
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

func artifactHandlingOpaqueChoices() ([]OpaqueModelChoice, error) {
	definitions := []struct {
		description string
		value       string
	}{
		{"The artifact must remain unchanged.", string(ArtifactPreserveUnchanged)},
		{"The artifact's existence is required.", string(ArtifactMustExist)},
		{"The artifact's absence is explicitly required.", string(ArtifactMustBeAbsent)},
		{"The artifact is one member of an unresolved explicit absence choice.", string(ArtifactPossibleAbsenceCandidate)},
		{"The artifact is referenced without another explicit state relation.", string(ArtifactMentionedOnly)},
	}
	choices := make([]OpaqueModelChoice, 0, len(definitions))
	for _, definition := range definitions {
		choice, err := NewOpaqueModelChoice(definition.description, definition.value)
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}
