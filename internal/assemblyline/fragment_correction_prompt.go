package assemblyline

import (
	"fmt"
	"strings"
)

// BuildFragmentCorrectionPrompt renders the execution half of repair. Its
// model-visible authority is exactly one instruction and one mutable source
// value; parsing and signature/capability validation remain code-owned.
func BuildFragmentCorrectionPrompt(input FragmentCorrectionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	mutable := strings.TrimSpace(input.CurrentDeclaration)
	if input.RepairRegion != nil {
		mutable = input.RepairRegion.Source
	}
	encoded, err := marshalUntrustedPromptString(mutable)
	if err != nil {
		return "", fmt.Errorf("encode exact mutable source: %w", err)
	}
	prompt := strings.Join([]string{
		"Apply one repair instruction to one exact mutable source declaration or region.",
		"Return only its raw replacement source: no JSON, Markdown, line numbers, explanation, imports, or additional declarations.",
		"EXACT_MUTABLE_SOURCE_JSON:\n" + encoded,
		"REQUIRED_SOURCE_TRANSFORMATION:\n" + input.RepairGuidance,
	}, "\n\n")
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf("fragment correction prompt exceeds %d bytes", maxPortableResourceBytes)
	}
	return prompt, nil
}
