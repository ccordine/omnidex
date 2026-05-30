package queue

import (
	"strings"
	"unicode/utf8"
)

// SanitizeUTF8Text replaces invalid UTF-8 byte sequences so PostgreSQL text/jsonb
// columns accept the value (invalid bytes otherwise raise SQLSTATE 22021).
func SanitizeUTF8Text(s string) string {
	if s == "" {
		return s
	}
	if strings.Contains(s, "\x00") {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "\uFFFD")
}

// SanitizeUTF8Bytes ensures b is valid UTF-8, preserving JSON structure when possible.
func SanitizeUTF8Bytes(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	out := SanitizeUTF8Text(string(b))
	return []byte(out)
}
