package assemblyline

import (
	"fmt"
	"strings"
)

// BuildBoundedSourceFragmentGenerationPrompt renders one path-blind source
// declaration question for a registered bounded source language
// parser. It grants no document, placement, operation, or completion authority.
func BuildBoundedSourceFragmentGenerationPrompt(
	input FragmentGenerationInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	language, err := boundedSourceLanguageByID(input.Language)
	if err != nil {
		return "", err
	}
	if _, err := validateBoundedSourceFragment(
		language, input.Signature, input.Signature+" {}",
	); err != nil {
		return "", fmt.Errorf("validate exact %s fragment signature: %w", language.display, err)
	}
	prompt := strings.Join([]string{
		"The complete response grammar is exactly one raw " + language.display + " " + language.declaration +
			" with the exact declared signature and one body.",
		"Implement only the exact local behavior. The declaration's identifier vocabulary consists of language-predeclared values and the explicitly listed direct capabilities and permitted symbols.",
		"SOURCE_DIALECT:\n" + input.Dialect,
		"EXACT_SIGNATURE:\n" + input.Signature,
		"EXACT_LOCAL_BEHAVIOR:\n" + input.Behavior,
		"DIRECT_CAPABILITIES:\n" + strings.Join(input.Capabilities, "\n"),
		"PERMITTED_SYMBOLS:\n" + strings.Join(input.PermittedSymbols, "\n"),
	}, "\n\n")
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf(
			"%s fragment generation prompt exceeds %d bytes",
			language.display, maxPortableResourceBytes,
		)
	}
	return prompt, nil
}
