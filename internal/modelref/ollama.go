package modelref

import (
	"fmt"
	"regexp"
	"strings"
)

const MaxOllamaNameBytes = 256

var ollamaNamePattern = regexp.MustCompile(`^[A-Za-z0-9._:/@-]+$`)

// ValidateOllamaName validates identifier syntax only. It does not inspect,
// select, load, or invoke a provider model.
func ValidateOllamaName(name string) error {
	if name == "" || name != strings.TrimSpace(name) || len(name) > MaxOllamaNameBytes ||
		!ollamaNamePattern.MatchString(name) {
		return fmt.Errorf("Ollama model must be canonical exact text of at most %d bytes", MaxOllamaNameBytes)
	}
	return nil
}
