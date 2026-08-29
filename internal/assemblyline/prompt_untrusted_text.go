package assemblyline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// marshalUntrustedPromptString preserves one exact value while preventing its
// bytes from becoming prompt section markers or provider control tokens.
func marshalUntrustedPromptString(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("untrusted prompt string is not valid UTF-8")
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", fmt.Errorf("encode untrusted prompt string: %w", err)
	}
	encoded := strings.TrimSuffix(buffer.String(), "\n")
	// Ollama's raw tokenizer recognizes provider controls such as
	// <|endoftext|>. Escaping only their opening preserves ordinary source
	// characters (including JSX) while JSON decoding still recovers exact bytes.
	encoded = strings.ReplaceAll(encoded, "<|", `\u003c|`)
	return encoded, nil
}
