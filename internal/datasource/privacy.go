package datasource

import (
	"strings"
)

func IsSensitiveColumn(name string) bool {
	return IsIdentifierColumn(name) || isClinicalNoteColumn(name)
}

func SensitiveColumns(columns []string) []string {
	out := []string{}
	for _, col := range columns {
		if IsSensitiveColumn(col) || IsTextContentColumn(col, "") {
			out = append(out, col)
		}
	}
	return out
}

// ValidatePrivacySafeSelect rejects queries that project sensitive columns without aggregation.
func ValidatePrivacySafeSelect(sql string, strict bool) error {
	if !strict {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(sql))
	if lower == "" {
		return nil
	}
	if isTextSampleQuery(sql) {
		return nil
	}
	// Allow COUNT/aggregate patterns even if sensitive names appear inside.
	if strings.Contains(lower, " count(") || strings.Contains(lower, "count(*)") || strings.Contains(lower, " group by ") {
		return nil
	}
	for _, pattern := range identifierColumnPatterns {
		if pattern.MatchString(lower) {
			return errSensitiveProjection
		}
	}
	for _, pattern := range textContentColumnPatterns {
		if pattern.MatchString(lower) {
			return errTextProjection
		}
	}
	if strings.Contains(lower, "clinical") && strings.Contains(lower, "note") {
		return errSensitiveProjection
	}
	return nil
}

var errTextProjection = privacyError("query may expose patient text; use COUNT/GROUP BY aggregates, or a bounded text sample (LIMIT <= 24) for sentiment analysis")

var errSensitiveProjection = privacyError("query may expose sensitive columns; use aggregates (COUNT, GROUP BY) instead")

type privacyError string

func (e privacyError) Error() string { return string(e) }
