package worker

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const objectiveEvidenceTruncationMarker = "\n...[truncated]"

func boundedObjectiveEvidenceText(maximum int, values ...string) (string, bool, error) {
	if maximum <= len(objectiveEvidenceTruncationMarker) {
		return "", false, fmt.Errorf("objective evidence bound %d cannot represent truncation", maximum)
	}
	trimmed := make([]string, 0, len(values))
	required := 0
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return "", false, fmt.Errorf("objective evidence projection contains invalid text")
		}
		if len(trimmed) > 0 {
			required++
		}
		required += len(value)
		trimmed = append(trimmed, value)
	}
	if len(trimmed) == 0 {
		return "", false, fmt.Errorf("objective evidence projection is blank")
	}
	limit := maximum
	truncated := required > maximum
	if truncated {
		limit -= len(objectiveEvidenceTruncationMarker)
	}
	var result strings.Builder
	result.Grow(maximum)
	for index, value := range trimmed {
		if index > 0 && result.Len() < limit {
			result.WriteByte('\n')
		}
		remaining := limit - result.Len()
		if remaining <= 0 {
			break
		}
		if len(value) > remaining {
			value = value[:remaining]
			for len(value) > 0 && !utf8.ValidString(value) {
				value = value[:len(value)-1]
			}
		}
		result.WriteString(value)
	}
	if truncated {
		result.WriteString(objectiveEvidenceTruncationMarker)
	}
	return result.String(), truncated, nil
}
