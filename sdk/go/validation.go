package omnidex

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	maxPromptBytes   = 4 * 1024
	maxResponseBytes = 16 * 1024 * 1024
)

func validateClientConfiguration(baseURL, token string) error {
	if baseURL == "" || baseURL != strings.TrimSpace(baseURL) {
		return fmt.Errorf("Omnidex base URL must be exact nonblank text")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("Omnidex base URL must be one HTTP(S) origin or canonical base path")
	}
	if parsed.Path != "" && (path.Clean(parsed.Path) != parsed.Path || strings.HasSuffix(parsed.Path, "/")) {
		return fmt.Errorf("Omnidex base URL path must be canonical without a trailing slash")
	}
	if err := validateToken(token); err != nil {
		return err
	}
	return nil
}

func validateToken(token string) error {
	if len(token) < 32 || len(token) > 4096 {
		return fmt.Errorf("Omnidex integration token must contain 32..4096 exact ASCII bytes")
	}
	for _, character := range []byte(token) {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("Omnidex integration token must contain only visible ASCII bytes")
		}
	}
	return nil
}

func validateCanonicalID(label, value string, maximum int) error {
	if value == "" || len(value) > maximum {
		return fmt.Errorf("%s must contain 1..%d canonical bytes", label, maximum)
	}
	for index, character := range []byte(value) {
		allowed := character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '_' || character == '.' || character == ':' || character == '-'
		if !allowed || index == 0 && !((character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9')) {
			return fmt.Errorf("%s is not canonical", label)
		}
	}
	return nil
}

func ValidateDelegatedAuthorityID(value string) error {
	if len(value) != 68 || !strings.HasPrefix(value, "dba_") {
		return fmt.Errorf("delegated authority must be one opaque dba_ identity")
	}
	decoded, err := hex.DecodeString(value[4:])
	if err != nil || len(decoded) != 32 || value[4:] != strings.ToLower(value[4:]) {
		return fmt.Errorf("delegated authority must be one opaque dba_ identity")
	}
	return nil
}

func NewDelegatedAuthorityID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate delegated authority identity: %w", err)
	}
	return "dba_" + hex.EncodeToString(value), nil
}

func validatePrompt(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > maxPromptBytes || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("prompt must contain 1..%d exact nonblank UTF-8 bytes without NUL", maxPromptBytes)
	}
	return nil
}
