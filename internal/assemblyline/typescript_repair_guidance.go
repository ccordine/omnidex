package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxTypeScriptRepairGuidanceBytes = 2 * 1024

// TypeScriptRepairGuidanceInput is the complete input for deriving one bounded
// source-transformation instruction from one exact diagnostic.
type TypeScriptRepairGuidanceInput struct {
	Language           string                             `json:"language"`
	Dialect            string                             `json:"dialect"`
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

// FragmentRepairGuidanceInput is the language-neutral repair-analysis
// envelope. The TypeScript spelling names the same schema because its optional
// compiler-region evidence is TypeScript-specific; there is one implementation.
type FragmentRepairGuidanceInput = TypeScriptRepairGuidanceInput
type FragmentRepairGuidanceRejection = TypeScriptRepairGuidanceRejection
type FragmentRepairGuidanceRejectionKind = TypeScriptRepairGuidanceRejectionKind
type FragmentRepairGuidance = TypeScriptRepairGuidance

const (
	FragmentRepairGuidanceNoSourceChange      = TypeScriptRepairGuidanceNoSourceChange
	FragmentRepairGuidanceRepeatedInstruction = TypeScriptRepairGuidanceRepeatedInstruction
)

func NewFragmentRepairGuidanceJob(input FragmentRepairGuidanceInput) (PortableJob, error) {
	return NewTypeScriptRepairGuidanceJob(input)
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

func DecodeFragmentRepairGuidanceResult(
	job PortableJob,
	raw string,
) (FragmentRepairGuidance, error) {
	return DecodeTypeScriptRepairGuidanceResult(job, raw)
}

func (input TypeScriptRepairGuidanceInput) validate() error {
	if _, err := boundedSourceLanguageByID(input.Language); err != nil && input.Language != "typescript" {
		return fmt.Errorf("fragment repair guidance: %w", err)
	}
	if err := validatePortableFragmentCore(
		input.Language, input.Signature, input.Capabilities, input.PermittedSymbols,
	); err != nil {
		return err
	}
	if input.Dialect == "" || input.Dialect != strings.TrimSpace(input.Dialect) ||
		strings.ContainsAny(input.Dialect, "\x00\r\n") || len(input.Dialect) > 256 {
		return fmt.Errorf("fragment repair guidance dialect is required as one bounded label")
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
		if input.Language != "typescript" {
			return fmt.Errorf("fragment repair guidance regions require TypeScript")
		}
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
	proseValues := []string{input.Dialect, input.Diagnostic}
	sourceValues := []string{input.Signature, input.CurrentDeclaration}
	sourceValues = append(sourceValues, input.Capabilities...)
	sourceValues = append(sourceValues, input.PermittedSymbols...)
	if input.RepairRegion != nil {
		sourceValues = append(sourceValues, input.RepairRegion.Source)
		for _, binding := range append(
			append([]TypeScriptRepairBinding(nil), input.RepairRegion.Bindings...),
			input.RepairRegion.UnavailableBindings...,
		) {
			sourceValues = append(sourceValues, binding.Name, binding.Type)
			sourceValues = append(sourceValues, binding.CallableSignatures...)
			sourceValues = append(sourceValues, binding.Members...)
		}
		for _, evidence := range input.RepairRegion.ExpressionEvidence {
			sourceValues = append(sourceValues, evidence.Source, evidence.InferredType, evidence.ContextualType)
			sourceValues = append(sourceValues, evidence.IncompatibleTypes...)
			sourceValues = append(sourceValues, evidence.ReferencedBindings...)
		}
	}
	if input.PriorRejection != nil {
		proseValues = append(proseValues, input.PriorRejection.Instruction)
	}
	if err := ValidatePathFreeModelContext("TypeScript repair guidance", proseValues...); err != nil {
		return err
	}
	return ValidatePathFreeSourceModelContext("TypeScript repair guidance", sourceValues...)
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
	if err := ValidatePathFreeModelContext(
		"TypeScript repair guidance result", guidance.Instruction,
	); err != nil {
		return err
	}
	return nil
}

func (guidance TypeScriptRepairGuidance) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	return ValidatePathFreeModelContextWithProvenance(
		"TypeScript repair guidance result", provenance, guidance.Instruction,
	)
}

func TypeScriptRepairGuidanceResponseSchema() map[string]any {
	return objectSchema([]string{"instruction"}, map[string]any{
		"instruction": map[string]any{
			"type": "string", "minLength": 1,
		},
	})
}

func FragmentRepairGuidanceResponseSchema() map[string]any {
	return TypeScriptRepairGuidanceResponseSchema()
}
