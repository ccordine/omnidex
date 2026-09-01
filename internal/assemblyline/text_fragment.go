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
	prompt := "Write text that fulfills this request.\n\n" + input.Behavior
	if len(prompt) > maxPortableResourceBytes {
		return "", fmt.Errorf("text fragment generation prompt exceeds %d bytes", maxPortableResourceBytes)
	}
	return prompt, nil
}

// NormalizeTextFragmentResponse applies the code-owned line-ending contract to
// an ordinary text response. Provider formatting is not a model qualification
// responsibility.
func NormalizeTextFragmentResponse(raw string) (string, error) {
	if raw == "" || !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("text response must be non-empty UTF-8 without NUL bytes")
	}
	if len(raw) > maxPortableCandidateBytes {
		return "", fmt.Errorf("text response exceeds %d bytes", maxPortableCandidateBytes)
	}
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimRight(normalized, "\n") + "\n"
	if normalized == "\n" {
		return "", fmt.Errorf("text response contains no content")
	}
	return normalized, nil
}

// ValidateTextFragment validates a text node after code has applied its
// document framing. It is not a response format imposed on the model.
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
