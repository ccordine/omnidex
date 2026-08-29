package repository

import "strings"

// GeneratedGoSource recognizes the canonical leading marker used by Go
// generators. It is deterministic source authority shared by candidate
// preflight and exact staged mutation validation.
func GeneratedGoSource(content []byte) bool {
	for _, line := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "// Code generated ") &&
			strings.HasSuffix(trimmed, " DO NOT EDIT.") {
			return true
		}
		if !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "/*") &&
			!strings.HasPrefix(trimmed, "*") {
			return false
		}
	}
	return false
}
