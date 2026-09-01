package assemblyline

import (
	"fmt"
	"strings"
)

// BuildBoundedSourceFragmentGenerationPrompt renders one path-blind source
// body question for a registered bounded source language parser. Declaration
// structure is code-owned and is deliberately not part of the requested reply.
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
	if _, err := boundedSourceDeclarationShape(language, input.Signature+" {}"); err != nil {
		return "", fmt.Errorf("validate exact %s fragment signature: %w", language.display, err)
	}
	parts := []string{
		"What " + language.display + " statements implement this behavior?",
		input.Behavior,
		"The source dialect is " + input.Dialect + ".",
		"These parameters and return constraints are in scope:\n" + input.Signature,
	}
	if len(input.Capabilities) > 0 {
		parts = append(parts,
			"These direct declarations are available:\n"+strings.Join(input.Capabilities, "\n"),
		)
	}
	if len(input.PermittedSymbols) > 0 {
		parts = append(parts,
			"These additional identifiers are available:\n"+strings.Join(input.PermittedSymbols, "\n"),
		)
	}
	prompt := strings.Join(parts, "\n\n")
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf(
			"%s fragment generation prompt exceeds %d bytes",
			language.display, maxPortableResourceBytes,
		)
	}
	return prompt, nil
}
