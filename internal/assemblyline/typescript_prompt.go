package assemblyline

import (
	"fmt"
	"strings"
)

const (
	maxTypeScriptRequiredChangeBytes  = 512
	maxTypeScriptDiagnosticBytes      = 1024
	maxTypeScriptInitialEnvelopeBytes = maxPortableResourceBytes
	maxTypeScriptFragmentPromptBytes  = maxPortableResourceBytes
)

type TypeScriptFragmentPrompt struct {
	Signature      string
	Contract       string
	Available      string
	Globals        []string
	Current        string
	RepairRegion   *TypeScriptFragmentRepairRegion
	RequiredChange string
	Diagnostic     string
}

func BuildTypeScriptFragmentPrompt(input TypeScriptFragmentPrompt) (string, error) {
	signature := strings.TrimSpace(input.Signature)
	contract := strings.TrimSpace(input.Contract)
	available := strings.TrimSpace(input.Available)
	current := strings.TrimSpace(input.Current)
	hasRegion := input.RepairRegion != nil
	requiredChange := strings.TrimSpace(input.RequiredChange)
	diagnostic := strings.TrimSpace(input.Diagnostic)
	if signature == "" || strings.ContainsAny(signature, "\r\n") {
		return "", fmt.Errorf("TypeScript fragment prompt requires one single-line signature")
	}
	if contract == "" && current == "" && !hasRegion {
		return "", fmt.Errorf("TypeScript fragment prompt requires a local behavior contract")
	}
	if contract != "" && (current != "" || hasRegion) {
		return "", fmt.Errorf("TypeScript correction prompt cannot replay the initial behavior contract")
	}
	if current != "" && hasRegion {
		return "", fmt.Errorf("TypeScript correction prompt requires one current declaration or repair region")
	}
	if len(requiredChange) > maxTypeScriptRequiredChangeBytes {
		return "", fmt.Errorf("TypeScript fragment required change exceeds %d bytes", maxTypeScriptRequiredChangeBytes)
	}
	if len(diagnostic) > maxTypeScriptDiagnosticBytes {
		return "", fmt.Errorf("TypeScript fragment diagnostic exceeds %d bytes", maxTypeScriptDiagnosticBytes)
	}
	if current == "" && !hasRegion && (requiredChange != "" || diagnostic != "") {
		return "", fmt.Errorf("TypeScript fragment generation cannot carry correction fields")
	}
	if (current != "" || hasRegion) && (requiredChange == "" || diagnostic == "") {
		return "", fmt.Errorf("TypeScript fragment correction requires one change and diagnostic")
	}
	encodedCurrent := ""
	if current != "" {
		var err error
		encodedCurrent, err = marshalUntrustedPromptString(current)
		if err != nil {
			return "", fmt.Errorf("TypeScript fragment current declaration: %w", err)
		}
	}
	encodedRegion := ""
	if hasRegion {
		if err := input.RepairRegion.validate(); err != nil {
			return "", fmt.Errorf("TypeScript fragment repair region: %w", err)
		}
		encodedSource, err := marshalUntrustedPromptString(input.RepairRegion.Source)
		if err != nil {
			return "", fmt.Errorf("TypeScript fragment repair region: %w", err)
		}
		encodedRegion = fmt.Sprintf(
			`{"kind":%q,"start_line":%d,"end_line":%d,"source":%s}`,
			input.RepairRegion.Kind, input.RepairRegion.StartLine, input.RepairRegion.EndLine, encodedSource,
		)
	}
	parts := []string{}
	if hasRegion {
		parts = append(parts,
			"Repair one local region inside a TypeScript function declaration.",
			"Return only the raw replacement source for the selected region. Do not return JSON, line numbers, Markdown, explanation, or the full declaration.",
			"The enclosing declaration has this exact signature:\n"+signature,
		)
	} else {
		parts = append(parts,
			"Implement exactly one TypeScript function declaration.",
			"Return raw code only: no Markdown, import, export, surrounding explanation, or additional declaration.",
			"The declaration must match this signature exactly:\n"+signature,
		)
	}
	if contract != "" {
		parts = append(parts, "LOCAL_BEHAVIOR:\n"+contract)
	}
	if available != "" {
		parts = append(parts, "ONLY_AVAILABLE_DECLARATIONS:\n"+available)
	}
	if len(input.Globals) > 0 {
		parts = append(parts, "ALREADY_IN_SCOPE_IDENTIFIERS:\n"+strings.Join(input.Globals, ", "))
	}
	if current != "" {
		parts = append(parts, "CURRENT_DECLARATION_JSON:\n"+encodedCurrent)
	}
	if hasRegion {
		parts = append(parts, "CURRENT_REPAIR_REGION_JSON:\n"+encodedRegion)
	}
	if current != "" || hasRegion {
		parts = append(parts,
			"REQUIRED_CHANGE:\n"+requiredChange,
			"OBSERVED_FAILURE:\n"+diagnostic,
		)
		if hasRegion {
			parts = append(parts, fmt.Sprintf(
				"Return replacement source for only lines %d through %d. Do not return JSON, line numbers, Markdown, explanation, or the whole declaration.",
				input.RepairRegion.StartLine, input.RepairRegion.EndLine,
			))
		} else {
			parts = append(parts, "Return the corrected declaration only.")
		}
	}
	prompt := strings.Join(parts, "\n\n")
	if current == "" && !hasRegion && len(prompt) > maxTypeScriptInitialEnvelopeBytes {
		return "", fmt.Errorf("TypeScript fragment initial envelope exceeds %d bytes", maxTypeScriptInitialEnvelopeBytes)
	}
	if len(prompt) > maxTypeScriptFragmentPromptBytes {
		return "", fmt.Errorf("TypeScript fragment prompt exceeds %d bytes", maxTypeScriptFragmentPromptBytes)
	}
	return prompt, nil
}
