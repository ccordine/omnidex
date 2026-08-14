package assemblyline

import (
	"fmt"
	"strings"
)

func BuildGoFragmentCorrectionPrompt(input FragmentCorrectionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	if input.Language != "go" {
		return "", fmt.Errorf("Go fragment correction does not support language %q", input.Language)
	}
	current, err := marshalUntrustedPromptString(input.CurrentDeclaration)
	if err != nil {
		return "", fmt.Errorf("Go fragment correction current declaration: %w", err)
	}
	return strings.Join([]string{
		"Return exactly one raw Go function or method declaration with the exact signature. Do not use Markdown fences, imports, package clauses, comments, helper declarations, paths, files, commands, or explanations.",
		"Apply only the required change needed to resolve the exact diagnostic. Preserve all unrelated executable behavior in the current declaration.",
		"Use only identifiers already present in the current declaration or explicitly listed as permitted direct capabilities. Transitive repository symbols are unavailable.",
		"EXACT_SIGNATURE:\n" + input.Signature,
		"REQUIRED_CHANGE:\n" + input.RequiredChange,
		"EXACT_PATH_FREE_DIAGNOSTIC:\n" + input.Diagnostic,
		"CURRENT_DECLARATION_JSON:\n" + current,
		"DIRECT_CAPABILITIES:\n" + strings.Join(input.Capabilities, "\n"),
		"PERMITTED_SYMBOLS:\n" + strings.Join(input.PermittedSymbols, "\n"),
	}, "\n\n"), nil
}
