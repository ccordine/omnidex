package assemblyline

import (
	"fmt"
	"strings"
)

func renderPortableFragmentGeneration(input FragmentGenerationInput) (string, error) {
	switch input.Language {
	case "go":
		return BuildGoFragmentGenerationPrompt(input)
	case TextFragmentLanguage:
		return BuildTextFragmentGenerationPrompt(input)
	case "typescript":
		return BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
			Dialect:   input.Dialect,
			Signature: input.Signature,
			Contract:  input.Behavior,
			Available: strings.Join(input.Capabilities, "\n"),
			Globals:   input.PermittedSymbols,
		})
	case "javascript", "java", "rust", "php":
		return BuildBoundedSourceFragmentGenerationPrompt(input)
	default:
		return "", fmt.Errorf("no fragment renderer supports language %q", input.Language)
	}
}

func renderPortableFragmentCorrection(input FragmentCorrectionInput) (string, error) {
	return BuildFragmentCorrectionPrompt(input)
}
