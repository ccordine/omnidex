package worker

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

var directCodingTypeScriptCompilerIssuePattern = regexp.MustCompile(
	`(?m)(?:^|[ \t])((?:\./)?[A-Za-z0-9_./-]+\.tsx?)(?::([0-9]+):([0-9]+)|\(([0-9]+),([0-9]+)\))(?::)?[ \t]+([^\r\n]+)`,
)

type directCodingTypeScriptCompilerIssue struct {
	path    string
	line    int
	column  int
	message string
}

func mapDirectCodingTypeScriptStageDiagnostic(
	documents []assemblyline.ComposedSourceDocument,
	output string,
) (*directCodingStageDiagnostic, bool) {
	paths := make([]string, len(documents))
	for index, document := range documents {
		paths[index] = document.Path
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(paths)
	if err != nil {
		return nil, false
	}
	searchable := directCodingANSISequencePattern.ReplaceAllString(output, "")
	for _, issue := range directCodingTypeScriptCompilerIssues(searchable) {
		diagnostic, mapped := mapDirectCodingTypeScriptDocumentLocation(
			documents, issue.path, issue.line, issue.column, searchable,
		)
		if !mapped {
			continue
		}
		diagnostic.ModelFeedback = directCodingTypeScriptLocatedCompilerFailure(
			diagnostic.DeclarationLine, diagnostic.DeclarationColumn, issue.message, provenance,
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

func directCodingTypeScriptLocatedCompilerFailure(
	line int,
	column int,
	message string,
	provenance assemblyline.ArtifactIdentityProvenance,
) string {
	message = redactDirectCodingPathIdentities(strings.TrimSpace(message), provenance)
	if line < 1 || column < 1 || message == "" ||
		modelcontext.ContainsPathIdentityWithProvenance(message, provenance) {
		return ""
	}
	return fmt.Sprintf(
		"DECLARATION_LOCATION: line %d column %d\nTYPESCRIPT_DIAGNOSTIC: %s",
		line, column, message,
	)
}

func directCodingTypeScriptCompilerContainsPathIdentity(value string) bool {
	return modelcontext.ContainsPathIdentity(value)
}
