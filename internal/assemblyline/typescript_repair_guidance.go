package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxTypeScriptRepairGuidanceBytes = 2 * 1024

// TypeScriptRepairGuidanceInput is the complete diagnostic authority for one
// repair analyst call. The later source executor never receives this envelope.
type TypeScriptRepairGuidanceInput struct {
	Language           string                          `json:"language"`
	Signature          string                          `json:"signature"`
	Capabilities       []string                        `json:"capabilities"`
	PermittedSymbols   []string                        `json:"permitted_symbols"`
	CurrentDeclaration string                          `json:"current_declaration,omitempty"`
	RepairRegion       *TypeScriptFragmentRepairRegion `json:"repair_region,omitempty"`
	Diagnostic         string                          `json:"diagnostic"`
}

// TypeScriptRepairGuidance is one instruction-only semantic leaf. It has no
// source-code, routing, mutation, verification, or completion authority.
type TypeScriptRepairGuidance struct {
	Instruction string `json:"instruction"`
}

func NewTypeScriptRepairGuidanceJob(
	input TypeScriptRepairGuidanceInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkTypeScriptRepairGuidance, input, input.validate,
	)
}

func (input TypeScriptRepairGuidanceInput) validate() error {
	if input.Language != "typescript" {
		return fmt.Errorf("TypeScript repair guidance language must be %q", "typescript")
	}
	if err := validatePortableFragmentCore(
		input.Language, input.Signature, input.Capabilities, input.PermittedSymbols,
	); err != nil {
		return err
	}
	if (input.CurrentDeclaration == "") == (input.RepairRegion == nil) {
		return fmt.Errorf(
			"TypeScript repair guidance requires exactly one current declaration or repair region",
		)
	}
	if input.CurrentDeclaration != "" {
		if input.CurrentDeclaration != strings.TrimSpace(input.CurrentDeclaration) ||
			!utf8.ValidString(input.CurrentDeclaration) {
			return fmt.Errorf("TypeScript repair guidance current declaration must be trimmed UTF-8")
		}
	}
	if input.RepairRegion != nil {
		if err := input.RepairRegion.validate(); err != nil {
			return fmt.Errorf("TypeScript repair guidance region: %w", err)
		}
	}
	if input.Diagnostic == "" || input.Diagnostic != strings.TrimSpace(input.Diagnostic) {
		return fmt.Errorf("TypeScript repair guidance diagnostic is required and must be trimmed")
	}
	if len(input.Diagnostic) > maxTypeScriptDiagnosticBytes {
		return fmt.Errorf(
			"TypeScript repair guidance diagnostic exceeds %d bytes",
			maxTypeScriptDiagnosticBytes,
		)
	}
	return nil
}

func (guidance TypeScriptRepairGuidance) Validate() error {
	if guidance.Instruction == "" || guidance.Instruction != strings.TrimSpace(guidance.Instruction) ||
		!utf8.ValidString(guidance.Instruction) || strings.ContainsRune(guidance.Instruction, 0) {
		return fmt.Errorf("TypeScript repair guidance instruction must be trimmed valid UTF-8")
	}
	if len(guidance.Instruction) > maxTypeScriptRepairGuidanceBytes {
		return fmt.Errorf(
			"TypeScript repair guidance instruction exceeds %d bytes",
			maxTypeScriptRepairGuidanceBytes,
		)
	}
	return nil
}

func BuildTypeScriptRepairGuidancePrompt(
	input TypeScriptRepairGuidanceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	parts := []string{
		"Diagnose one exact TypeScript validation failure as a repair analyst. Do not write the replacement source.",
		"Return one self-contained imperative repair instruction for a separate source executor. The executor will receive only your instruction and the exact mutable source shown here; it will not receive the diagnostic, signature, declarations, symbol inventory, or any other context.",
		"Name every concrete identifier, expression, type conversion, scope move, and preservation constraint the executor needs. Resolve only the observed failure. Do not return Markdown, a replacement declaration, or a replacement region.",
		"REQUIRED_DECLARATION_SIGNATURE:\n" + input.Signature,
	}
	if len(input.Capabilities) > 0 {
		parts = append(parts,
			"DECLARATIONS_AVAILABLE_TO_ANALYZE:\n"+strings.Join(input.Capabilities, "\n"),
		)
	}
	if len(input.PermittedSymbols) > 0 {
		parts = append(parts,
			"IDENTIFIERS_ALREADY_IN_SCOPE:\n"+strings.Join(input.PermittedSymbols, ", "),
		)
	}
	if input.CurrentDeclaration != "" {
		encoded, err := marshalUntrustedPromptString(input.CurrentDeclaration)
		if err != nil {
			return "", fmt.Errorf("encode TypeScript repair-guidance declaration: %w", err)
		}
		parts = append(parts, "EXACT_MUTABLE_DECLARATION_JSON:\n"+encoded)
	} else {
		encoded, err := marshalUntrustedPromptString(input.RepairRegion.Source)
		if err != nil {
			return "", fmt.Errorf("encode TypeScript repair-guidance region: %w", err)
		}
		parts = append(parts, fmt.Sprintf(
			"EXACT_MUTABLE_REGION_JSON:\n{\"start_line\":%d,\"end_line\":%d,\"source\":%s}",
			input.RepairRegion.StartLine, input.RepairRegion.EndLine, encoded,
		))
		available, err := encodeTypeScriptRepairGuidanceBindings(input.RepairRegion.Bindings)
		if err != nil {
			return "", err
		}
		parts = append(parts, "BINDINGS_AVAILABLE_AT_FAILURE_JSON:\n"+available)
		unavailable, err := encodeTypeScriptRepairGuidanceBindings(
			input.RepairRegion.UnavailableBindings,
		)
		if err != nil {
			return "", err
		}
		parts = append(parts,
			"NESTED_BINDINGS_UNAVAILABLE_AT_FAILURE_JSON:\n"+unavailable,
			"An unavailable binding cannot be referenced at the failing expression. If it owns the needed value, instruct the executor to move the consuming operation into its lexical scope or derive the value from an explicitly available binding inside the mutable region.",
		)
	}
	parts = append(parts, "EXACT_VALIDATION_FAILURE:\n"+input.Diagnostic)
	prompt := strings.Join(parts, "\n\n")
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf(
			"TypeScript repair guidance prompt exceeds %d bytes", maxPortableResourceBytes,
		)
	}
	return prompt, nil
}

func encodeTypeScriptRepairGuidanceBindings(
	bindings []TypeScriptRepairBinding,
) (string, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(bindings); err != nil {
		return "", fmt.Errorf("encode TypeScript repair-guidance bindings: %w", err)
	}
	return strings.TrimSuffix(encoded.String(), "\n"), nil
}

func TypeScriptRepairGuidanceResponseSchema() map[string]any {
	return objectSchema([]string{"instruction"}, map[string]any{
		"instruction": map[string]any{
			"type": "string", "minLength": 1,
			"maxLength": maxTypeScriptRepairGuidanceBytes,
		},
	})
}
