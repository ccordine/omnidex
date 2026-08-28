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

func renderPortableResponseCorrection(input ResponseCorrectionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	originalPrompt, err := RenderPortableJob(input.Original)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"The prior raw semantic leaf failed one exact deterministic validation. Answer the same single semantic question with one complete replacement leaf that resolves only that defect.",
		"Return only the complete replacement leaf in the original raw format. Do not return a patch, diff, JSON wrapper, quotes, label, Markdown, explanation, or control instruction.",
		"ORIGINAL_SEMANTIC_QUESTION:\n" + originalPrompt,
		"CURRENT_REJECTED_LEAF:\n" + input.RetainedCandidate,
		"EXACT_GROUNDED_DEFECT:\n" + input.ValidationFailure,
	}, "\n\n"), nil
}
