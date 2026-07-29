package api

import "strings"

const scrumChannelThoughtMaxChars = 280

func mergeScrumChannelThoughtText(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return compactScrumChannelText(next, scrumChannelThoughtMaxChars)
	}
	if next == "" {
		return existing
	}
	return compactScrumChannelText(existing+" · "+next, scrumChannelThoughtMaxChars)
}

func compactScrumChannelText(raw string, maxChars int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if text == "" || maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	const marker = " … "
	if maxChars <= len(marker)+12 {
		return text[:maxChars]
	}
	head := (maxChars - len(marker)) * 2 / 3
	tail := maxChars - len(marker) - head
	return text[:head] + marker + text[len(text)-tail:]
}
