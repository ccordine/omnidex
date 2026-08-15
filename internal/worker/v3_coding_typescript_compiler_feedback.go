package worker

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var directCodingTypeScriptCompilerIssuePattern = regexp.MustCompile(
	`(?m)(?:^|[ \t])((?:\./)?[A-Za-z0-9_./-]+\.tsx?)(?::([0-9]+):([0-9]+)|\(([0-9]+),([0-9]+)\))(?::)?[ \t]+([^\r\n]+)`,
)

var directCodingTypeScriptCompilerFileIdentityPattern = regexp.MustCompile(
	`(?i)(?:^|[^[:alnum:]_])[[:alnum:]_-][[:alnum:]_.-]*\.(?:tsx?|jsx?|mjs|cjs|json|map)(?:$|[^[:alnum:]_.-])`,
)

type directCodingTypeScriptCompilerIssue struct {
	path    string
	line    int
	column  int
	message string
}

func mapDirectCodingTypeScriptStageDiagnostic(
	documents []assemblyline.ComposedTypeScriptDocument,
	output string,
) (*directCodingStageDiagnostic, bool) {
	searchable := directCodingANSISequencePattern.ReplaceAllString(output, "")
	for _, issue := range directCodingTypeScriptCompilerIssues(searchable) {
		diagnostic, mapped := mapDirectCodingTypeScriptDocumentLocation(
			documents, issue.path, issue.line, issue.column, searchable,
		)
		if !mapped {
			continue
		}
		diagnostic.ModelFeedback = directCodingTypeScriptLocatedCompilerFailure(
			diagnostic.DeclarationLine, diagnostic.DeclarationColumn, issue.message,
		)
		diagnostic.CompilerIssue = true
		return diagnostic, true
	}
	return nil, false
}

func directCodingTypeScriptCompilerIssues(output string) []directCodingTypeScriptCompilerIssue {
	clean := directCodingANSISequencePattern.ReplaceAllString(strings.ReplaceAll(output, "\r", ""), "")
	matches := directCodingTypeScriptCompilerIssuePattern.FindAllStringSubmatch(clean, -1)
	issues := make([]directCodingTypeScriptCompilerIssue, 0, len(matches))
	for _, match := range matches {
		lineRaw, columnRaw := match[2], match[3]
		if lineRaw == "" {
			lineRaw, columnRaw = match[4], match[5]
		}
		line, lineErr := strconv.Atoi(lineRaw)
		column, columnErr := strconv.Atoi(columnRaw)
		message := strings.TrimSpace(match[6])
		if lineErr != nil || columnErr != nil || line < 1 || column < 1 || message == "" {
			continue
		}
		issues = append(issues, directCodingTypeScriptCompilerIssue{
			path: filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(match[1]), "./"), "/")),
			line: line, column: column, message: message,
		})
	}
	return issues
}

func directCodingTypeScriptLocatedCompilerFailure(line int, column int, message string) string {
	message = directCodingTypeScriptIdentityPattern.ReplaceAllString(strings.TrimSpace(message), "[source]")
	if line < 1 || column < 1 || message == "" || directCodingTypeScriptCompilerContainsPathIdentity(message) {
		return ""
	}
	return fmt.Sprintf(
		"DECLARATION_LOCATION: line %d column %d\nTYPESCRIPT_DIAGNOSTIC: %s",
		line, column, message,
	)
}

func directCodingTypeScriptCompilerContainsPathIdentity(value string) bool {
	if directCodingTypeScriptCompilerFileIdentityPattern.MatchString(value) {
		return true
	}
	for _, field := range strings.Fields(value) {
		token := strings.Trim(field, `"'()[]{}<>,;:`)
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "./") ||
			strings.HasPrefix(token, "../") || strings.ContainsAny(token, `/\`) {
			return true
		}
		if len(token) >= 3 && token[1] == ':' && (token[2] == '/' || token[2] == '\\') {
			return true
		}
	}
	return false
}
