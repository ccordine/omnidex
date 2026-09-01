package main

import "strings"

func truncateForWatch(value string, maxChars int) string {
	text := strings.TrimSpace(value)
	if maxChars <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "\n...[truncated]"
}

func compactProgressValue(value string, maxChars int) string {
	text := strings.TrimSpace(strings.ReplaceAll(value, "\n", " | "))
	if maxChars <= 0 || maxChars > 320 {
		maxChars = 320
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "...[truncated]"
}

func indentBlock(value, prefix string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n")
}
