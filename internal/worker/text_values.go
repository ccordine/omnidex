package worker

import (
	"fmt"
	"strings"
)

func parseAnyStringSlice(value any) []string {
	if items, ok := value.([]any); ok {
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	}
	if value == nil {
		return nil
	}
	if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
		return []string{text}
	}
	return nil
}

func splitCSVTags(value string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, raw := range strings.Split(value, ",") {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

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

func appendUnique(base []string, values ...string) []string {
	out := make([]string, 0, len(base)+len(values))
	seen := make(map[string]struct{}, len(base)+len(values))
	for _, value := range append(append([]string(nil), base...), values...) {
		clean := strings.ToLower(strings.TrimSpace(value))
		if clean == "" {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func csvOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ",")
}
