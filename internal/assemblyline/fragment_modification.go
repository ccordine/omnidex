package assemblyline

import (
	"fmt"
	"strings"
)

type FragmentModificationInput struct {
	Language           string   `json:"language"`
	Signature          string   `json:"signature"`
	CurrentDeclaration string   `json:"current_declaration"`
	RequirementQuote   string   `json:"requirement_quote"`
	Capabilities       []string `json:"capabilities"`
	PermittedSymbols   []string `json:"permitted_symbols"`
}

func NewFragmentModificationJob(input FragmentModificationInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkFragmentModification, input, input.validate)
}

func (input FragmentModificationInput) validate() error {
	if err := validatePortableFragmentCore(input.Language, input.Signature, input.Capabilities, input.PermittedSymbols); err != nil {
		return err
	}
	if input.Language != "go" {
		return fmt.Errorf("fragment modification does not support language %q", input.Language)
	}
	if input.CurrentDeclaration == "" || input.CurrentDeclaration != strings.TrimSpace(input.CurrentDeclaration) {
		return fmt.Errorf("fragment modification current declaration is required and must be trimmed")
	}
	if len(input.CurrentDeclaration) > maxTypeScriptCurrentDeclarationBytes {
		return fmt.Errorf("fragment modification current declaration exceeds %d bytes", maxTypeScriptCurrentDeclarationBytes)
	}
	if input.RequirementQuote == "" || input.RequirementQuote != strings.TrimSpace(input.RequirementQuote) {
		return fmt.Errorf("fragment modification requirement quote is required and must be trimmed")
	}
	if len(input.RequirementQuote) > maxLocalBehaviorBytes {
		return fmt.Errorf("fragment modification requirement quote exceeds %d bytes", maxLocalBehaviorBytes)
	}
	return nil
}

func BuildGoFragmentModificationPrompt(input FragmentModificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return exactly one raw Go function or method declaration with the exact signature. Do not use Markdown fences, imports, package clauses, comments, helper declarations, paths, files, commands, or explanations.",
		"Apply only the exact requirement quote and preserve unrelated executable behavior in the current declaration.",
		"Use only identifiers already present in the current declaration or explicitly listed as permitted direct capabilities. Transitive repository symbols are unavailable.",
		"EXACT_SIGNATURE:\n" + input.Signature,
		"EXACT_REQUIREMENT_QUOTE:\n" + input.RequirementQuote,
		"CURRENT_DECLARATION:\n" + input.CurrentDeclaration,
		"DIRECT_CAPABILITIES:\n" + strings.Join(input.Capabilities, "\n"),
		"PERMITTED_SYMBOLS:\n" + strings.Join(input.PermittedSymbols, "\n"),
	}, "\n\n"), nil
}
