package worker

import (
	"strings"
)

func safeLine(value, fallback string) string {
	clean := strings.Join(strings.Fields(value), " ")
	if clean == "" {
		return fallback
	}
	return clean
}

func trimForBudget(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if maxChars <= 0 || len(runes) <= maxChars {
		return value
	}
	if maxChars < 20 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-15]) + "\n...[truncated]"
}
