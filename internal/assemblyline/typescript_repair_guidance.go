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
	Language           string                          `json:"language"`
	Dialect            string                          `json:"dialect"`
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

// FragmentRepairGuidanceInput is the language-neutral repair-analysis
// envelope. The TypeScript spelling names the same schema because its optional
// compiler-region evidence is TypeScript-specific; there is one implementation.
type FragmentRepairGuidanceInput = TypeScriptRepairGuidanceInput
type FragmentRepairGuidance = TypeScriptRepairGuidance

func NewFragmentRepairGuidanceJob(input FragmentRepairGuidanceInput) (PortableJob, error) {
	return NewTypeScriptRepairGuidanceJob(input)
}

func NewTypeScriptRepairGuidanceJob(
	input TypeScriptRepairGuidanceInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkTypeScriptRepairGuidance, input,
	)
}

// DecodeTypeScriptRepairGuidanceResult applies the same raw-leaf and value
// validation used by the production semantic worker to one untrusted guidance
// response. Replay and qualification callers must not invent a weaker decoder.
func DecodeTypeScriptRepairGuidanceResult(
	job PortableJob,
	raw string,
) (TypeScriptRepairGuidance, error) {
	var guidance TypeScriptRepairGuidance
	if job.Kind != WorkTypeScriptRepairGuidance {
		return guidance, fmt.Errorf(
			"TypeScript repair-guidance result requires work kind %q",
			WorkTypeScriptRepairGuidance,
		)
	}
	leaf, err := decodeRawSemanticLeaf(
		"TypeScript repair guidance", raw, maxTypeScriptRepairGuidanceBytes, true,
	)
	if err != nil {
		return guidance, err
	}
	guidance = TypeScriptRepairGuidance{Instruction: leaf}
	if err := guidance.Validate(); err != nil {
		return guidance, err
	}
	var input FragmentRepairGuidanceInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return guidance, err
	}
	if err := validateFragmentRepairGuidanceInstruction(input, guidance.Instruction); err != nil {
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
	if err := validateFragmentRepairGuidanceLanguage(input.Language); err != nil {
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
	if err := ValidatePathFreeModelContext("TypeScript repair guidance", proseValues...); err != nil {
		return err
	}
	return ValidatePathFreeSourceModelContext("TypeScript repair guidance", sourceValues...)
}

func validateFragmentRepairGuidanceLanguage(language string) error {
	if language == "go" || language == "typescript" {
		return nil
	}
	_, err := boundedSourceLanguageByID(language)
	return err
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
	if err := ValidatePathFreeRepairInstructionModelContext(
		"TypeScript repair guidance result", guidance.Instruction,
	); err != nil {
		return err
	}
	return nil
}

func (guidance TypeScriptRepairGuidance) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	return ValidatePathFreeRepairInstructionModelContextWithProvenance(
		"TypeScript repair guidance result", provenance, guidance.Instruction,
	)
}
