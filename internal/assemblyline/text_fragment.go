package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	TextFragmentLanguage  = "text"
	TextFragmentDialect   = "UTF-8 text with LF line endings and exactly one terminal LF"
	TextFragmentSignature = "plain_utf8_text_node"
)

// BuildTextFragmentGenerationPrompt renders one path-blind text-node question.
// Document identity, placement, framing, and workspace operations are absent.
func BuildTextFragmentGenerationPrompt(input FragmentGenerationInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	if input.Language != TextFragmentLanguage {
		return "", fmt.Errorf("text fragment generation does not support language %q", input.Language)
	}
	if input.Dialect != TextFragmentDialect {
		return "", fmt.Errorf("text fragment generation requires dialect %q", TextFragmentDialect)
	}
	if input.Signature != TextFragmentSignature {
		return "", fmt.Errorf("text fragment generation requires signature %q", TextFragmentSignature)
	}
	if len(input.Capabilities) != 0 || len(input.PermittedSymbols) != 0 {
		return "", fmt.Errorf("text fragment generation cannot receive source capabilities or symbols")
	}
	prompt := strings.Join([]string{
		"The complete response grammar is exactly one raw UTF-8 text node.",
		"Return only the node bytes without a label, quotation, JSON wrapper, or Markdown wrapper.",
		"Use LF bytes for line endings. End the node with exactly one LF byte. Do not return a carriage return or NUL byte.",
		"Implement only the exact local behavior.",
		"TEXT_DIALECT:\n" + input.Dialect,
		"TEXT_NODE_GRAMMAR:\n" + input.Signature,
		"EXACT_LOCAL_BEHAVIOR:\n" + input.Behavior,
	}, "\n\n")
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf("text fragment generation prompt exceeds %d bytes", maxPortableResourceBytes)
	}
	return prompt, nil
}

// ValidateTextFragment enforces the exact raw-node transport grammar. It does
// not trim, normalize, or reconstruct any accepted byte.
func ValidateTextFragment(raw string) error {
	if raw == "" {
		return fmt.Errorf("text fragment is empty")
	}
	if len(raw) > maxPortableCandidateBytes {
		return fmt.Errorf("text fragment exceeds %d bytes", maxPortableCandidateBytes)
	}
	if !utf8.ValidString(raw) {
		return fmt.Errorf("text fragment must be valid UTF-8")
	}
	if strings.ContainsRune(raw, '\x00') {
		return fmt.Errorf("text fragment contains NUL")
	}
	if strings.ContainsRune(raw, '\r') {
		return fmt.Errorf("text fragment must use LF line endings")
	}
	if !strings.HasSuffix(raw, "\n") {
		return fmt.Errorf("text fragment must end with one terminal LF")
	}
	if len(raw) == 1 || strings.HasSuffix(raw, "\n\n") {
		return fmt.Errorf("text fragment must contain content and exactly one terminal LF")
	}
	return nil
}

// ProjectTextFragment binds the validated node to the complete provider
// response. No prefix, suffix, or formatting transport may be discarded.
func ProjectTextFragment(raw string) (PortableResultProjection, error) {
	if err := ValidateTextFragment(raw); err != nil {
		return PortableResultProjection{}, err
	}
	return NewExactPortableResultProjection(raw)
}
