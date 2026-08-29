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
	return strings.Join([]string{
		"The complete response grammar is exactly one raw Go function declaration with the exact signature and one body.",
		"Implement only the exact local behavior. The declaration's identifier vocabulary consists of predeclared Go identifiers and identifiers explicitly listed as permitted direct capabilities.",
		"SOURCE_DIALECT:\n" + input.Dialect,
		"EXACT_SIGNATURE:\n" + input.Signature,
		"EXACT_LOCAL_BEHAVIOR:\n" + input.Behavior,
		"DIRECT_CAPABILITIES:\n" + strings.Join(input.Capabilities, "\n"),
		"PERMITTED_SYMBOLS:\n" + strings.Join(input.PermittedSymbols, "\n"),
	}, "\n\n"), nil
}
