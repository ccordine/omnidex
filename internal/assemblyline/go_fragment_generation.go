package assemblyline

import (
	"fmt"
	"strings"
)

func BuildGoFragmentGenerationPrompt(input FragmentGenerationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	if input.Language != "go" {
		return "", fmt.Errorf("Go fragment generation does not support language %q", input.Language)
	}
	parts := []string{
		"The source dialect is " + input.Dialect + ".",
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
	parts = append(parts,
		input.Behavior,
		"These parameters and return constraints are in scope:\n"+input.Signature,
		"What Go statements should execute inside this function?",
	)
	return strings.Join(parts, "\n\n"), nil
}
