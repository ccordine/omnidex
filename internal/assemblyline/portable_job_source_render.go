package assemblyline

import (
	"fmt"
	"strings"
)

func renderPortableFragmentGeneration(input FragmentGenerationInput) (string, error) {
	var prompt string
	var err error
	switch input.Language {
	case "go":
		prompt, err = BuildGoFragmentGenerationPrompt(input)
	case TextFragmentLanguage:
		prompt, err = BuildTextFragmentGenerationPrompt(input)
	case "typescript":
		prompt, err = BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
			Dialect:   input.Dialect,
			Signature: input.Signature,
			Contract:  input.Behavior,
			Available: strings.Join(input.Capabilities, "\n"),
			Globals:   input.PermittedSymbols,
		})
	case "javascript", "java", "rust":
		prompt, err = BuildBoundedSourceFragmentGenerationPrompt(input)
	default:
		return "", fmt.Errorf("no fragment renderer supports language %q", input.Language)
	}
	if err != nil {
		return "", err
	}
	return prompt, nil
}

func renderPortableFragmentGenerationReplacement(
	input FragmentGenerationReplacementInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return renderPortableFragmentGeneration(input.Original)
}

func renderPortableFragmentCorrection(input FragmentCorrectionInput) (string, error) {
	return BuildFragmentCorrectionPrompt(input)
}
