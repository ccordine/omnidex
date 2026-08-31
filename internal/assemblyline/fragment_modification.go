package assemblyline

import (
	"fmt"
	"strings"
)

type FragmentModificationInput struct {
	Language           string   `json:"language"`
	Dialect            string   `json:"dialect"`
	Signature          string   `json:"signature"`
	CurrentDeclaration string   `json:"current_declaration"`
	RequirementQuote   string   `json:"requirement_quote"`
	Capabilities       []string `json:"capabilities"`
	PermittedSymbols   []string `json:"permitted_symbols"`
}

func NewFragmentModificationJob(input FragmentModificationInput) (PortableJob, error) {
	return newPortableJob(WorkFragmentModification, input)
}

func (input FragmentModificationInput) validate() error {
	if err := validatePortableFragmentCore(input.Language, input.Signature, input.Capabilities, input.PermittedSymbols); err != nil {
		return err
	}
	if input.Language != "go" {
		return fmt.Errorf("fragment modification does not support language %q", input.Language)
	}
	if input.Dialect == "" || input.Dialect != strings.TrimSpace(input.Dialect) ||
		strings.ContainsAny(input.Dialect, "\x00\r\n") || len(input.Dialect) > 256 {
		return fmt.Errorf("fragment modification dialect is required as one bounded label")
	}
	if input.CurrentDeclaration == "" || input.CurrentDeclaration != strings.TrimSpace(input.CurrentDeclaration) {
		return fmt.Errorf("fragment modification current declaration is required and must be trimmed")
	}
	if input.RequirementQuote == "" || input.RequirementQuote != strings.TrimSpace(input.RequirementQuote) {
		return fmt.Errorf("fragment modification requirement quote is required and must be trimmed")
	}
	if len(input.RequirementQuote) > maxLocalBehaviorBytes {
		return fmt.Errorf("fragment modification requirement quote exceeds %d bytes", maxLocalBehaviorBytes)
	}
	return input.ValidatePathFree(ArtifactIdentityProvenance{})
}

// ValidatePathFree keeps the human requirement on the strict prose boundary
// and validates only parser-proven declaration/capability fields as source.
func (input FragmentModificationInput) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	if err := ValidatePathFreeModelContextWithProvenance(
		"fragment modification requirement", provenance, input.Dialect, input.RequirementQuote,
	); err != nil {
		return err
	}
	sourceValues := []string{input.Signature, input.CurrentDeclaration}
	sourceValues = append(sourceValues, input.Capabilities...)
	sourceValues = append(sourceValues, input.PermittedSymbols...)
	return ValidatePathFreeSourceModelContextWithProvenance(
		"fragment modification", provenance, sourceValues...,
	)
}

func BuildGoFragmentModificationPrompt(input FragmentModificationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	current, err := marshalUntrustedPromptString(input.CurrentDeclaration)
	if err != nil {
		return "", fmt.Errorf("Go fragment modification current declaration: %w", err)
	}
	return strings.Join([]string{
		"The complete response grammar is exactly one raw Go function or method declaration with the exact signature and one body.",
		"Apply only the exact requirement quote and preserve unrelated executable behavior in the current declaration.",
		"The declaration's identifier vocabulary consists of predeclared Go identifiers, identifiers already present in the current declaration, and identifiers explicitly listed as permitted direct capabilities.",
		"SOURCE_DIALECT:\n" + input.Dialect,
		"EXACT_SIGNATURE:\n" + input.Signature,
		"EXACT_REQUIREMENT_QUOTE:\n" + input.RequirementQuote,
		"CURRENT_DECLARATION_JSON:\n" + current,
		"DIRECT_CAPABILITIES:\n" + strings.Join(input.Capabilities, "\n"),
		"PERMITTED_SYMBOLS:\n" + strings.Join(input.PermittedSymbols, "\n"),
	}, "\n\n"), nil
}
