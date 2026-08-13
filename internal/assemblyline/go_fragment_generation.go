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
		"Return exactly one raw Go function declaration with the exact signature. Do not use Markdown fences, imports, package clauses, comments, methods, helper declarations, paths, files, commands, operations, or explanations.",
		"Implement only the exact local behavior. Use only predeclared Go identifiers or identifiers explicitly listed as permitted direct capabilities.",
		"EXACT_SIGNATURE:\n" + input.Signature,
		"EXACT_LOCAL_BEHAVIOR:\n" + input.Behavior,
		"DIRECT_CAPABILITIES:\n" + strings.Join(input.Capabilities, "\n"),
		"PERMITTED_SYMBOLS:\n" + strings.Join(input.PermittedSymbols, "\n"),
	}, "\n\n"), nil
}
