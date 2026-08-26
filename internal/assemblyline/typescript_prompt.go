package assemblyline

import (
	"fmt"
	"strings"
)

const (
	maxTypeScriptDiagnosticBytes      = 1024
	maxTypeScriptInitialEnvelopeBytes = maxPortableResourceBytes
	maxTypeScriptFragmentPromptBytes  = maxPortableResourceBytes
)

type TypeScriptFragmentPrompt struct {
	Dialect        string
	Signature      string
	Contract       string
	Available      string
	Globals        []string
	Current        string
	RepairRegion   *TypeScriptFragmentRepairRegion
	RequiredChange string
	Diagnostic     string
	RepairGuidance string
}

func BuildTypeScriptFragmentPrompt(input TypeScriptFragmentPrompt) (string, error) {
	signature := strings.TrimSpace(input.Signature)
	contract := strings.TrimSpace(input.Contract)
	available := strings.TrimSpace(input.Available)
	current := strings.TrimSpace(input.Current)
	hasRegion := input.RepairRegion != nil
	repairGuidance := strings.TrimSpace(input.RepairGuidance)
	dialect := strings.TrimSpace(input.Dialect)
	if signature == "" || strings.ContainsAny(signature, "\r\n") {
		return "", fmt.Errorf("TypeScript fragment prompt requires one single-line signature")
	}
	if strings.TrimSpace(input.RequiredChange) != "" || strings.TrimSpace(input.Diagnostic) != "" {
		return "", fmt.Errorf(
			"unguided TypeScript fragment correction is forbidden; derive one repair instruction first",
		)
	}
	if current != "" && hasRegion {
		return "", fmt.Errorf("TypeScript correction prompt requires one current declaration or repair region")
	}
	if current != "" || hasRegion {
		if repairGuidance == "" {
			return "", fmt.Errorf(
				"unguided TypeScript fragment correction is forbidden; derive one repair instruction first",
			)
		}
		if dialect != "" || contract != "" || available != "" || len(input.Globals) != 0 {
			return "", fmt.Errorf(
				"guided TypeScript correction cannot receive diagnostic-analysis context",
			)
		}
		if len(repairGuidance) > maxTypeScriptRepairGuidanceBytes {
			return "", fmt.Errorf(
				"TypeScript repair guidance exceeds %d bytes",
				maxTypeScriptRepairGuidanceBytes,
			)
		}
		return buildGuidedTypeScriptRepairPrompt(input, current, hasRegion, repairGuidance)
	}
	if repairGuidance != "" {
		return "", fmt.Errorf("TypeScript fragment generation cannot carry repair guidance")
	}
	if contract == "" {
		return "", fmt.Errorf("TypeScript fragment prompt requires a local behavior contract")
	}
	if dialect == "" || dialect != input.Dialect || strings.ContainsAny(dialect, "\x00\r\n") || len(dialect) > 256 {
		return "", fmt.Errorf("TypeScript fragment prompt requires one bounded source dialect")
	}
	parts := []string{
		"Implement exactly one TypeScript function declaration.",
		"Return raw code only: no Markdown, import, export, surrounding explanation, or additional declaration.",
		"SOURCE_DIALECT:\n" + dialect,
		"The declaration must match this signature exactly:\n" + signature,
		"LOCAL_BEHAVIOR:\n" + contract,
	}
	if available != "" {
		parts = append(parts, "ONLY_AVAILABLE_DECLARATIONS:\n"+available)
	}
	if len(input.Globals) > 0 {
		parts = append(parts, "ALREADY_IN_SCOPE_IDENTIFIERS:\n"+strings.Join(input.Globals, ", "))
	}
	prompt := strings.Join(parts, "\n\n")
	if len(prompt) > maxTypeScriptInitialEnvelopeBytes {
		return "", fmt.Errorf(
			"TypeScript fragment initial envelope exceeds %d bytes",
			maxTypeScriptInitialEnvelopeBytes,
		)
	}
	return prompt, nil
}

func buildGuidedTypeScriptRepairPrompt(
	input TypeScriptFragmentPrompt,
	current string,
	hasRegion bool,
	repairGuidance string,
) (string, error) {
	mutable := current
	if hasRegion {
		if err := input.RepairRegion.validate(); err != nil {
			return "", fmt.Errorf("guided TypeScript repair region: %w", err)
		}
		mutable = input.RepairRegion.Source
	}
	encoded, err := marshalUntrustedPromptString(mutable)
	if err != nil {
		return "", fmt.Errorf("guided TypeScript mutable source: %w", err)
	}
	output := "Return one corrected raw TypeScript function declaration."
	if hasRegion {
		output = "Return one raw TypeScript replacement for this exact source region."
	}
	prompt := strings.Join([]string{
		output + " Return source only: no JSON, Markdown, line numbers, explanation, imports, or unrelated declarations.",
		"EXACT_MUTABLE_SOURCE_JSON:\n" + encoded,
		"REQUIRED_SOURCE_TRANSFORMATION:\n" + repairGuidance,
	}, "\n\n")
	if len(prompt) > maxTypeScriptFragmentPromptBytes {
		return "", fmt.Errorf(
			"guided TypeScript repair prompt exceeds %d bytes",
			maxTypeScriptFragmentPromptBytes,
		)
	}
	return prompt, nil
}
