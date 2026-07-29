package assemblyline

import (
	"fmt"
	"strings"
)

const (
	maxTypeScriptCapabilityBytes         = 2 * 1024
	maxTypeScriptCurrentDeclarationBytes = 5 * 1024
	maxTypeScriptRequiredChangeBytes     = 512
	maxTypeScriptDiagnosticBytes         = 1024
	maxTypeScriptInitialEnvelopeBytes    = 3 * 1024
	maxTypeScriptFragmentPromptBytes     = 8 * 1024
)

type TypeScriptFragmentPrompt struct {
	Signature      string
	Contract       string
	Available      string
	Globals        []string
	Current        string
	RequiredChange string
	Diagnostic     string
}

func BuildTypeScriptFragmentPrompt(input TypeScriptFragmentPrompt) (string, error) {
	signature := strings.TrimSpace(input.Signature)
	contract := strings.TrimSpace(input.Contract)
	available := strings.TrimSpace(input.Available)
	current := strings.TrimSpace(input.Current)
	requiredChange := strings.TrimSpace(input.RequiredChange)
	diagnostic := strings.TrimSpace(input.Diagnostic)
	if signature == "" || strings.ContainsAny(signature, "\r\n") {
		return "", fmt.Errorf("TypeScript fragment prompt requires one single-line signature")
	}
	if contract == "" && current == "" {
		return "", fmt.Errorf("TypeScript fragment prompt requires a local behavior contract")
	}
	if contract != "" && current != "" {
		return "", fmt.Errorf("TypeScript correction prompt cannot replay the initial behavior contract")
	}
	if len(available) > maxTypeScriptCapabilityBytes {
		return "", fmt.Errorf("TypeScript fragment capabilities exceed %d bytes", maxTypeScriptCapabilityBytes)
	}
	if len(current) > maxTypeScriptCurrentDeclarationBytes {
		return "", fmt.Errorf("TypeScript fragment current declaration exceeds %d bytes", maxTypeScriptCurrentDeclarationBytes)
	}
	if len(requiredChange) > maxTypeScriptRequiredChangeBytes {
		return "", fmt.Errorf("TypeScript fragment required change exceeds %d bytes", maxTypeScriptRequiredChangeBytes)
	}
	if len(diagnostic) > maxTypeScriptDiagnosticBytes {
		return "", fmt.Errorf("TypeScript fragment diagnostic exceeds %d bytes", maxTypeScriptDiagnosticBytes)
	}
	if current == "" && (requiredChange != "" || diagnostic != "") {
		return "", fmt.Errorf("TypeScript fragment generation cannot carry correction fields")
	}
	if current != "" && (requiredChange == "" || diagnostic == "") {
		return "", fmt.Errorf("TypeScript fragment current declaration requires one change and diagnostic")
	}
	parts := []string{
		"Implement exactly one TypeScript function declaration.",
		"Return raw code only: no Markdown, import, export, comments, surrounding explanation, or additional declaration.",
		"The declaration must match this signature exactly:\n" + signature,
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
		parts = append(parts, "CURRENT_DECLARATION:\n"+current)
	}
	if current != "" {
		parts = append(parts,
			"REQUIRED_CHANGE:\n"+requiredChange,
			"OBSERVED_FAILURE:\n"+diagnostic,
			"Return the corrected declaration only.",
		)
	}
	prompt := strings.Join(parts, "\n\n")
	if current == "" && len(prompt) > maxTypeScriptInitialEnvelopeBytes {
		return "", fmt.Errorf("TypeScript fragment initial envelope exceeds %d bytes", maxTypeScriptInitialEnvelopeBytes)
	}
	if len(prompt) > maxTypeScriptFragmentPromptBytes {
		return "", fmt.Errorf("TypeScript fragment prompt exceeds %d bytes", maxTypeScriptFragmentPromptBytes)
	}
	return prompt, nil
}
