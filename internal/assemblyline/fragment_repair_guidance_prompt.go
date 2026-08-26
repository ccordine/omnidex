package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

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
		"The instruction must be complete because only it and the exact mutable source will be available when it is applied. Name every required expression change and preservation constraint. Resolve only the observed failure.",
		"SOURCE_LANGUAGE:\n" + input.Language,
		"SOURCE_DIALECT:\n" + input.Dialect,
		"REQUIRED_DECLARATION_SIGNATURE:\n" + input.Signature,
	}
	if len(input.Capabilities) > 0 {
		parts = append(parts, "DECLARATIONS_AVAILABLE_TO_ANALYZE:\n"+strings.Join(input.Capabilities, "\n"))
	}
	if len(input.PermittedSymbols) > 0 {
		parts = append(parts, "IDENTIFIERS_ALREADY_IN_SCOPE:\n"+strings.Join(input.PermittedSymbols, ", "))
	}
	if input.CurrentDeclaration != "" {
		encoded, err := marshalUntrustedPromptString(input.CurrentDeclaration)
		if err != nil {
			return "", fmt.Errorf("encode repair-guidance declaration: %w", err)
		}
		parts = append(parts, "EXACT_MUTABLE_DECLARATION_JSON:\n"+encoded)
	} else if err := appendTypeScriptRepairRegionPrompt(&parts, input.RepairRegion); err != nil {
		return "", err
	}
	if input.PriorRejection != nil {
		encoded, err := marshalUntrustedPromptString(input.PriorRejection.Instruction)
		if err != nil {
			return "", fmt.Errorf("encode rejected repair instruction: %w", err)
		}
		parts = append(parts, "REJECTED_INSTRUCTION_JSON:\n"+encoded)
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
			return "", fmt.Errorf("render unsupported repair-guidance rejection %q", input.PriorRejection.Failure)
		}
	}
	parts = append(parts, "EXACT_VALIDATION_FAILURE:\n"+input.Diagnostic)
	prompt := strings.Join(parts, "\n\n")
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf("fragment repair guidance prompt exceeds %d bytes", maxPortableResourceBytes)
	}
	return prompt, nil
}

func BuildFragmentRepairGuidancePrompt(input FragmentRepairGuidanceInput) (string, error) {
	return BuildTypeScriptRepairGuidancePrompt(input)
}

func appendTypeScriptRepairRegionPrompt(
	parts *[]string,
	region *TypeScriptFragmentRepairRegion,
) error {
	encoded, err := marshalUntrustedPromptString(region.Source)
	if err != nil {
		return fmt.Errorf("encode repair-guidance region: %w", err)
	}
	*parts = append(*parts, fmt.Sprintf(
		"EXACT_MUTABLE_REGION_JSON:\n{\"start_line\":%d,\"end_line\":%d,\"source\":%s}",
		region.StartLine, region.EndLine, encoded,
	))
	available, err := encodeTypeScriptRepairGuidanceValue(region.Bindings)
	if err != nil {
		return err
	}
	*parts = append(*parts, "BINDINGS_AVAILABLE_AT_FAILURE_JSON:\n"+available)
	if len(region.ExpressionEvidence) > 0 {
		evidence, err := encodeTypeScriptRepairGuidanceValue(region.ExpressionEvidence)
		if err != nil {
			return err
		}
		*parts = append(*parts,
			"COMPILER_EXPRESSION_EVIDENCE_JSON:\n"+evidence,
			"Every incompatible_types entry must be absent from the replacement expression's possible result type.",
		)
	}
	if len(region.UnavailableBindings) > 0 {
		unavailable, err := encodeTypeScriptRepairGuidanceValue(region.UnavailableBindings)
		if err != nil {
			return err
		}
		*parts = append(*parts,
			"NESTED_BINDINGS_UNAVAILABLE_AT_FAILURE_JSON:\n"+unavailable,
			"An unavailable binding cannot be referenced at the failing expression. If it owns the needed value, the required transformation must move the consuming operation into its lexical scope or derive the value from an explicitly available binding inside the mutable region.",
		)
	}
	return nil
}

func encodeTypeScriptRepairGuidanceValue(value any) (string, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", fmt.Errorf("encode TypeScript repair-guidance evidence: %w", err)
	}
	return strings.TrimSuffix(encoded.String(), "\n"), nil
}
