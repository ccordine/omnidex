package main

import (
	"strings"
	"unicode"
)

func splitTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.ToLower(strings.TrimSpace(part))
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func safeValue(value, fallback string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return fallback
	}
	return clean
}

func normalizeForMatch(value string) string {
	var normalized strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
			continue
		}
		normalized.WriteByte(' ')
	}
	return strings.Join(strings.Fields(normalized.String()), " ")
}
