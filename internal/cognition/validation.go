package cognition

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func validateIdentity(value, field string) error {
	if value == "" || len(value) > MaxIdentityBytes || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%w: %s must be a nonempty bounded UTF-8 identifier", ErrInvalidIdentity, field)
	}
	for index, character := range value {
		if unicode.IsSpace(character) || !validIdentityRune(character, index == 0) {
			return fmt.Errorf("%w: %s contains an unregistered character", ErrInvalidIdentity, field)
		}
	}
	return nil
}

func validIdentityRune(character rune, first bool) bool {
	if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' {
		return true
	}
	if first {
		return false
	}
	switch character {
	case '-', '_', '.', ':', '/':
		return true
	default:
		return false
	}
}

func validateVersion(value, field string) error {
	if value == "" || len(value) > MaxVersionBytes {
		return fmt.Errorf("%w: %s must be a nonempty bounded version", ErrInvalidIdentity, field)
	}
	return validateIdentity(value, field)
}

func validateExactText(value, field string, maxBytes int) error {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must be one nonempty bounded exact UTF-8 value", field)
	}
	return nil
}

func validateContent(value, field string, maxBytes int) error {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must contain bounded UTF-8 content", field)
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func contentSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
