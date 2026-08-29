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
			Dialect:                  input.Dialect,
			Signature:                input.Signature,
			Contract:                 input.Behavior,
			Available:                strings.Join(input.Capabilities, "\n"),
			Globals:                  input.PermittedSymbols,
			PublicInteractionSurface: input.PublicInteractionSurface,
		})
	case "javascript", "java", "rust", "php":
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
	prompt, err := renderPortableFragmentGeneration(input.Original)
	if err != nil {
		return "", err
	}
	prompt +=
		"\n\nEXACT_OUTPUT_LIMIT_EVIDENCE:\n" +
			"A prior answer for this unchanged declaration reached the provider output boundary and contained no accepted declaration. " +
			"Return one complete declaration within this response; do not emit duplicate or redundant statements."
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf(
			"fragment generation replacement prompt exceeds %d bytes",
			maxPortableResourceBytes,
		)
	}
	return prompt, nil
}

func renderPortableFragmentCorrection(input FragmentCorrectionInput) (string, error) {
	return BuildFragmentCorrectionPrompt(input)
}
