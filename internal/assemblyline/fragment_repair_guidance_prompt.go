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
		"Return only the raw imperative instruction with no JSON, quotes, label, Markdown wrapper, or commentary.",
		"The instruction must be complete because only it and the exact mutable source will be available when it is applied. Name every required expression change and preservation constraint. Resolve only the observed failure.",
		"Constrain the instruction to replacing only the exact mutable source. Preserve REQUIRED_DECLARATION_SIGNATURE exactly. Do not require imports, package/module declarations, sibling declarations, or any other change outside that source.",
	}
	if input.CurrentDeclaration != "" {
		parts = append(parts,
			"Identifiers declared inside the exact mutable source and language-predeclared identifiers remain available. The two external-authority lists below are exhaustive. An external identifier merely referenced by the rejected source is unavailable unless one of those lists declares it.",
		)
	} else {
		parts = append(parts,
			"BINDINGS_AVAILABLE_AT_FAILURE_JSON, language-predeclared identifiers, and the two external-authority lists below are the exhaustive authority available inside the exact mutable region. A binding listed as unavailable or merely referenced without appearing in that authority cannot be used at the failing location.",
		)
	}
	parts = append(parts, "SOURCE_LANGUAGE:\n"+input.Language,
		"SOURCE_DIALECT:\n"+input.Dialect,
		"REQUIRED_DECLARATION_SIGNATURE:\n"+input.Signature,
		"DECLARATIONS_AVAILABLE_TO_ANALYZE:\n"+renderFragmentRepairGuidanceList(input.Capabilities, "\n"),
		"IDENTIFIERS_ALREADY_IN_SCOPE:\n"+renderFragmentRepairGuidanceList(input.PermittedSymbols, ", "),
	)
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
		case TypeScriptRepairGuidanceInvalidSource:
			parts = append(parts,
				"EXACT_INSTRUCTION_FAILURE:\nThe rejected instruction caused the source executor to return a replacement outside the code-owned declaration, signature, or identifier boundary.",
				"REQUIRED_INSTRUCTION_DELTA:\nReturn a different instruction whose complete change stays inside the exact mutable source and uses only the exhaustive authority shown above.",
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

func renderFragmentRepairGuidanceList(values []string, separator string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, separator)
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
