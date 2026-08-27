package assemblyline

import (
	"fmt"
	"strings"
)

func renderPortableFragmentGeneration(input FragmentGenerationInput) (string, map[string]any, error) {
	switch input.Language {
	case "go":
		prompt, err := BuildGoFragmentGenerationPrompt(input)
		return prompt, nil, err
	case "typescript":
		prompt, err := BuildTypeScriptFragmentPrompt(TypeScriptFragmentPrompt{
			Dialect:   input.Dialect,
			Signature: input.Signature,
			Contract:  input.Behavior,
			Available: strings.Join(input.Capabilities, "\n"),
			Globals:   input.PermittedSymbols,
		})
		return prompt, nil, err
	case "javascript", "java", "rust", "php":
		prompt, err := BuildBoundedSourceFragmentGenerationPrompt(input)
		return prompt, nil, err
	default:
		return "", nil, fmt.Errorf("no fragment renderer supports language %q", input.Language)
	}
}

func renderPortableFragmentCorrection(input FragmentCorrectionInput) (string, map[string]any, error) {
	prompt, err := BuildFragmentCorrectionPrompt(input)
	return prompt, nil, err
}

func renderPortableResponseCorrection(input ResponseCorrectionInput) (string, map[string]any, error) {
	schema, err := responseCorrectionSchema(input.Original, input.TargetField)
	if err != nil {
		return "", nil, err
	}
	if input.Original.Kind == WorkApplicationJobSpecification {
		prompt, promptErr := buildApplicationJobSpecificationResponseCorrectionPrompt(input)
		return prompt, schema, promptErr
	}
	if input.RetainedCandidate != "" {
		originalPrompt, _, promptErr := RenderPortableJob(input.Original)
		if promptErr != nil {
			return "", nil, promptErr
		}
		properties := schema["properties"].(map[string]any)
		var target string
		for field := range properties {
			target = field
		}
		return strings.Join([]string{
			"Return a JSON merge patch containing only the " + target + " field. Replace that one invalid semantic leaf while preserving every retained field.",
			"ORIGINAL_SEMANTIC_QUESTION:\n" + originalPrompt,
			"CURRENT_INVALID_RESPONSE:\n" + input.RetainedCandidate,
			"EXACT_VALIDATION_DEFECT:\n" + input.ValidationFailure,
		}, "\n\n"), schema, nil
	}
	return "", nil, fmt.Errorf(
		"%s response correction cannot render without one exact retained candidate",
		input.Original.Kind,
	)
}
