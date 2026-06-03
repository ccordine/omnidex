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

// TruncateUTF8Text returns a valid UTF-8 prefix plus suffix without cutting a
// multi-byte rune. maxPrefixBytes is the maximum byte length before suffix.
func TruncateUTF8Text(s string, maxPrefixBytes int, suffix string) string {
	s = SanitizeUTF8Text(s)
	if maxPrefixBytes <= 0 || len(s) <= maxPrefixBytes {
		return s
	}
	suffix = SanitizeUTF8Text(suffix)
	cut := 0
	for i, r := range s {
		next := i + utf8.RuneLen(r)
		if next > maxPrefixBytes {
			break
		}
		cut = next
	}
	return strings.TrimSpace(s[:cut]) + suffix
}
