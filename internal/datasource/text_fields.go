package datasource

import (
	"regexp"
	"strings"
)

const (
	PrivacyStandard   = "standard"
	MaxTextSampleRows = 24
)

var identifierColumnPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(ssn|social.?security|mrn|medical.?record|patient.?name|first.?name|last.?name|full.?name|email|phone|mobile|address|street|zip|postal|dob|date.?of.?birth|birth.?date|password|secret|token|api.?key)`),
}

var textContentColumnPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(comment|feedback|review|submission|submitted|survey|message|complaint|suggestion|patient.?input|response.?text|open.?ended|free.?text|narrative|testimonial|verbatim|patient.?note|intake.?note|portal.?message)`),
}

var textContentDataTypes = []string{
	"text",
	"character varying",
	"varchar",
	"json",
	"jsonb",
}

var textAnalysisQuestionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsentiment\b`),
	regexp.MustCompile(`(?i)\bcomment`),
	regexp.MustCompile(`(?i)\bfeedback\b`),
	regexp.MustCompile(`(?i)\breview`),
	regexp.MustCompile(`(?i)\bsubmitted\b`),
	regexp.MustCompile(`(?i)\bsubmission`),
	regexp.MustCompile(`(?i)\bsurvey\b`),
	regexp.MustCompile(`(?i)\bcomplaint`),
	regexp.MustCompile(`(?i)\bpatients?\s+(said|saying|report|reporting|mention|mentioning|submit|submitted|writing|wrote)\b`),
	regexp.MustCompile(`(?i)\b(open.?ended|free.?text|verbatim)\b`),
	regexp.MustCompile(`(?i)\bthemes?\b`),
	regexp.MustCompile(`(?i)\btone\b`),
	regexp.MustCompile(`(?i)\bwhat\s+are\s+patients\b`),
}

func IsIdentifierColumn(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, pattern := range identifierColumnPatterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}

func IsTextContentColumn(name, dataType string) bool {
	name = strings.TrimSpace(name)
	dataType = strings.ToLower(strings.TrimSpace(dataType))
	if name == "" {
		return false
	}
	if IsIdentifierColumn(name) || isClinicalNoteColumn(name) {
		return false
	}
	for _, pattern := range textContentColumnPatterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	for _, kind := range textContentDataTypes {
		if strings.Contains(dataType, kind) {
			if strings.Contains(name, "note") || strings.Contains(name, "comment") || strings.Contains(name, "body") || strings.Contains(name, "text") || strings.Contains(name, "message") {
				return true
			}
		}
	}
	return false
}

func isClinicalNoteColumn(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	return strings.Contains(name, "clinical") && strings.Contains(name, "note") ||
		strings.Contains(name, "progress_note") ||
		strings.Contains(name, "provider_note")
}

func WantsTextAnalysis(question string) bool {
	question = strings.TrimSpace(question)
	if question == "" {
		return false
	}
	for _, pattern := range textAnalysisQuestionPatterns {
		if pattern.MatchString(question) {
			return true
		}
	}
	return false
}

type TextFieldRef struct {
	Table    string `json:"table"`
	Column   string `json:"column"`
	DataType string `json:"data_type"`
	Purpose  string `json:"purpose,omitempty"`
}

func CatalogTextFields(catalog SchemaCatalog) []TextFieldRef {
	out := make([]TextFieldRef, 0, 16)
	for _, table := range catalog.Tables {
		for _, col := range table.Columns {
			if !IsTextContentColumn(col.Name, col.DataType) {
				continue
			}
			out = append(out, TextFieldRef{
				Table:    table.FullName,
				Column:   col.Name,
				DataType: col.DataType,
				Purpose:  col.Hint,
			})
		}
	}
	return out
}

func textContentColumns(columns []string, catalog SchemaCatalog, tableHint string) []string {
	out := make([]string, 0, len(columns))
	colTypes := map[string]string{}
	if table, ok := catalog.TableByName(tableHint); ok {
		for _, col := range table.Columns {
			colTypes[strings.ToLower(col.Name)] = col.DataType
		}
	}
	for _, col := range columns {
		dataType := colTypes[strings.ToLower(col)]
		if IsTextContentColumn(col, dataType) || (dataType == "" && IsTextContentColumn(col, "text")) {
			out = append(out, col)
		}
	}
	return out
}

func inferTableFromSQL(sql string, catalog SchemaCatalog) string {
	lower := strings.ToLower(sql)
	for _, table := range catalog.Tables {
		name := strings.ToLower(table.Name)
		full := strings.ToLower(table.FullName)
		if strings.Contains(lower, full) || strings.Contains(lower, " "+name+" ") || strings.Contains(lower, " "+name+"\n") {
			return table.FullName
		}
	}
	return ""
}

var limitClauseRe = regexp.MustCompile(`(?i)\blimit\s+(\d+)`)

func extractSQLLimit(sql string) int {
	match := limitClauseRe.FindStringSubmatch(sql)
	if len(match) < 2 {
		return 0
	}
	n := 0
	for _, ch := range match[1] {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func isTextSampleQuery(sql string) bool {
	limit := extractSQLLimit(sql)
	if limit <= 0 || limit > MaxTextSampleRows {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(sql))
	if !(strings.HasPrefix(lower, "select") || strings.HasPrefix(lower, "with")) {
		return false
	}
	for _, pattern := range identifierColumnPatterns {
		if pattern.MatchString(lower) {
			return false
		}
	}
	for _, pattern := range textContentColumnPatterns {
		if pattern.MatchString(lower) {
			return true
		}
	}
	for _, kind := range textContentDataTypes {
		if strings.Contains(lower, kind) {
			return strings.Contains(lower, "comment") || strings.Contains(lower, "feedback") || strings.Contains(lower, "message") || strings.Contains(lower, "body") || strings.Contains(lower, "text")
		}
	}
	return false
}
