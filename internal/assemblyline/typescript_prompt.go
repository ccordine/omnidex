package assemblyline

import (
	"fmt"
	"strings"
)

const maxTypeScriptInitialEnvelopeBytes = maxPortableResourceBytes

type TypeScriptFragmentPrompt struct {
	Dialect   string
	Signature string
	Contract  string
	Available string
	Globals   []string
}

func BuildTypeScriptFragmentPrompt(input TypeScriptFragmentPrompt) (string, error) {
	signature := strings.TrimSpace(input.Signature)
	contract := strings.TrimSpace(input.Contract)
	available := strings.TrimSpace(input.Available)
	dialect := strings.TrimSpace(input.Dialect)
	if signature == "" || strings.ContainsAny(signature, "\r\n") {
		return "", fmt.Errorf("TypeScript fragment prompt requires one single-line signature")
	}
	if contract == "" {
		return "", fmt.Errorf("TypeScript fragment prompt requires a local behavior contract")
	}
	if dialect == "" || dialect != input.Dialect || strings.ContainsAny(dialect, "\x00\r\n") || len(dialect) > 256 {
		return "", fmt.Errorf("TypeScript fragment prompt requires one bounded source dialect")
	}
	parts := []string{
		"What TypeScript statements implement this behavior?",
		contract,
		"This job is only the implementation itself; usage examples and documentation are not part of the task.",
		"The source dialect is " + dialect + ".",
		"These parameters and return constraints are in scope:\n" + signature,
	}
	if available != "" {
		parts = append(parts, "These direct declarations are available:\n"+available)
	}
	if len(input.Globals) > 0 {
		parts = append(parts, "These additional identifiers are available:\n"+strings.Join(input.Globals, ", "))
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
