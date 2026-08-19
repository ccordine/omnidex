package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxTypeScriptRepairGuidanceBytes = 2 * 1024

// TypeScriptRepairGuidanceInput is the complete input for deriving one bounded
// source-transformation instruction from one exact diagnostic.
type TypeScriptRepairGuidanceInput struct {
	Language           string                             `json:"language"`
	Signature          string                             `json:"signature"`
	Capabilities       []string                           `json:"capabilities"`
	PermittedSymbols   []string                           `json:"permitted_symbols"`
	CurrentDeclaration string                             `json:"current_declaration,omitempty"`
	RepairRegion       *TypeScriptFragmentRepairRegion    `json:"repair_region,omitempty"`
	Diagnostic         string                             `json:"diagnostic"`
	PriorRejection     *TypeScriptRepairGuidanceRejection `json:"prior_rejection,omitempty"`
}

// TypeScriptRepairGuidanceRejection is code-owned evidence that one previously
// accepted instruction produced no source transition for the exact current
// declaration and diagnostic.
type TypeScriptRepairGuidanceRejection struct {
	Instruction string                                `json:"instruction"`
	Failure     TypeScriptRepairGuidanceRejectionKind `json:"failure"`
}

type TypeScriptRepairGuidanceRejectionKind string

const (
	TypeScriptRepairGuidanceNoSourceChange      TypeScriptRepairGuidanceRejectionKind = "no_source_change"
	TypeScriptRepairGuidanceRepeatedInstruction TypeScriptRepairGuidanceRejectionKind = "repeated_instruction"
)

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

// DecodeTypeScriptRepairGuidanceResult applies the same closed JSON and value
// validation used by the production semantic worker to one untrusted guidance
// response. Replay and qualification callers must not invent a weaker decoder.
func DecodeTypeScriptRepairGuidanceResult(
	job PortableJob,
	raw string,
) (TypeScriptRepairGuidance, error) {
	var guidance TypeScriptRepairGuidance
	if err := job.Validate(); err != nil {
		return guidance, err
	}
	if job.Kind != WorkTypeScriptRepairGuidance {
		return guidance, fmt.Errorf(
			"TypeScript repair-guidance result requires work kind %q",
			WorkTypeScriptRepairGuidance,
		)
	}
	if err := decodePortablePayload([]byte(raw), &guidance); err != nil {
		return guidance, fmt.Errorf("decode TypeScript repair guidance: %w", err)
	}
	if err := guidance.Validate(); err != nil {
		return guidance, err
	}
	return guidance, nil
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
	if input.PriorRejection != nil {
		if err := input.PriorRejection.validate(); err != nil {
			return fmt.Errorf("TypeScript repair guidance prior rejection: %w", err)
		}
	}
	return nil
}

func (rejection TypeScriptRepairGuidanceRejection) validate() error {
	if rejection.Instruction == "" ||
		rejection.Instruction != strings.TrimSpace(rejection.Instruction) ||
		!utf8.ValidString(rejection.Instruction) || strings.ContainsRune(rejection.Instruction, 0) {
		return fmt.Errorf("instruction must be trimmed valid UTF-8")
	}
	if len(rejection.Instruction) > maxTypeScriptRepairGuidanceBytes {
		return fmt.Errorf("instruction exceeds %d bytes", maxTypeScriptRepairGuidanceBytes)
	}
	if rejection.Failure != TypeScriptRepairGuidanceNoSourceChange &&
		rejection.Failure != TypeScriptRepairGuidanceRepeatedInstruction {
		return fmt.Errorf("failure %q is unsupported", rejection.Failure)
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
	request := "Return one self-contained imperative source-transformation instruction that resolves EXACT_VALIDATION_FAILURE in the exact mutable source shown below."
	if input.PriorRejection != nil {
		request = "Return one replacement source-transformation instruction that changes the exact mutable source and resolves EXACT_VALIDATION_FAILURE."
	}
	parts := []string{
		request,
		"Name only the exact expression or expressions that must change and the required replacement semantics. Resolve only the observed failure.",
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
		expressionEvidence, err := encodeTypeScriptRepairGuidanceExpressionEvidence(
			input.RepairRegion.ExpressionEvidence,
		)
		if err != nil {
			return "", err
		}
		if len(input.RepairRegion.ExpressionEvidence) > 0 {
			parts = append(parts,
				"COMPILER_EXPRESSION_EVIDENCE_JSON:\n"+expressionEvidence,
				"Every incompatible_types entry must be absent from the replacement expression's possible result type.",
			)
		}
		if len(input.RepairRegion.UnavailableBindings) > 0 {
			unavailable, err := encodeTypeScriptRepairGuidanceBindings(
				input.RepairRegion.UnavailableBindings,
			)
			if err != nil {
				return "", err
			}
			parts = append(parts,
				"NESTED_BINDINGS_UNAVAILABLE_AT_FAILURE_JSON:\n"+unavailable,
				"An unavailable binding cannot be referenced at the failing expression. If it owns the needed value, the required transformation must move the consuming operation into its lexical scope or derive the value from an explicitly available binding inside the mutable region.",
			)
		}
	}
	if input.PriorRejection != nil {
		encoded, err := marshalUntrustedPromptString(input.PriorRejection.Instruction)
		if err != nil {
			return "", fmt.Errorf("encode rejected TypeScript repair instruction: %w", err)
		}
		parts = append(parts,
			"REJECTED_INSTRUCTION_JSON:\n"+encoded,
		)
		switch input.PriorRejection.Failure {
		case TypeScriptRepairGuidanceNoSourceChange:
			parts = append(parts,
				"EXACT_INSTRUCTION_FAILURE:\nThe rejected instruction produced a byte-identical replacement for the exact mutable source. It made no source change.",
				"REQUIRED_INSTRUCTION_DELTA:\nName a different replacement expression or operation. The current expression cannot be its own replacement.",
			)
		case TypeScriptRepairGuidanceRepeatedInstruction:
			parts = append(parts,
				"EXACT_INSTRUCTION_FAILURE:\nThe candidate repeated REJECTED_INSTRUCTION byte-for-byte after that instruction was already proven to make no source change.",
				"REQUIRED_INSTRUCTION_DELTA:\nReturn a different instruction naming a concrete source change that resolves EXACT_VALIDATION_FAILURE.",
			)
		default:
			return "", fmt.Errorf(
				"render unsupported TypeScript repair-guidance rejection %q",
				input.PriorRejection.Failure,
			)
		}
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

func encodeTypeScriptRepairGuidanceExpressionEvidence(
	evidence []TypeScriptRepairExpressionEvidence,
) (string, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(evidence); err != nil {
		return "", fmt.Errorf("encode TypeScript repair-guidance expression evidence: %w", err)
	}
	return strings.TrimSuffix(encoded.String(), "\n"), nil
}

func TypeScriptRepairGuidanceResponseSchema() map[string]any {
	return objectSchema([]string{"instruction"}, map[string]any{
		"instruction": map[string]any{
			"type": "string", "minLength": 1,
		},
	})
}
