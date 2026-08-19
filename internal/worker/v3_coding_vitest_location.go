package worker

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func mapDirectCodingVitestFailureReceipt(
	root string,
	documents []assemblyline.ComposedTypeScriptDocument,
	receipt directCodingVitestFailureReceipt,
) (*directCodingStageDiagnostic, bool, error) {
	for _, failure := range receipt.Failures {
		diagnostic, mapped, err := mapDirectCodingVitestFailureEvidence(root, documents, failure)
		if err != nil {
			return nil, false, err
		}
		if mapped {
			return diagnostic, true, nil
		}
	}
	return nil, false, nil
}

func mapDirectCodingVitestFailureEvidence(
	root string,
	documents []assemblyline.ComposedTypeScriptDocument,
	failure directCodingVitestFailureEvidence,
) (*directCodingStageDiagnostic, bool, error) {
	for _, location := range failure.Locations {
		path, valid := directCodingVitestStagePath(root, location.File)
		if !valid {
			continue
		}
		diagnostic, mapped := mapDirectCodingTypeScriptDocumentLocation(
			documents, path, location.Line, location.Column, failure.Output,
		)
		if !mapped {
			continue
		}
		diagnostic.FailureClass = failure.FailureClass
		feedback, err := directCodingTypeScriptStructuredTestModelFailure(
			failure, diagnostic.AuthorizedRegexLiterals...,
		)
		if err != nil {
			return nil, false, fmt.Errorf("map structured Vitest failure: %w", err)
		}
		diagnostic.ModelFeedback = feedback
		return diagnostic, true, nil
	}
	return nil, false, nil
}

func directCodingVitestStagePath(root string, raw string) (string, bool) {
	if root == "" || !filepath.IsAbs(root) {
		return "", false
	}
	file := strings.TrimSpace(raw)
	if file == "" {
		return "", false
	}
	if parsed, err := url.Parse(file); err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "file" || parsed.Path == "" {
			return "", false
		}
		file = parsed.Path
	}
	file = filepath.Clean(filepath.FromSlash(file))
	if filepath.IsAbs(file) {
		relative, err := filepath.Rel(filepath.Clean(root), file)
		if err != nil {
			return "", false
		}
		file = relative
	}
	if file == "." || file == ".." || strings.HasPrefix(file, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(file), true
}

func mapDirectCodingTypeScriptDocumentLocation(
	documents []assemblyline.ComposedTypeScriptDocument,
	path string,
	line int,
	column int,
	output string,
) (*directCodingStageDiagnostic, bool) {
	path = filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(path), "./"), "/"))
	if path == "" || line <= 0 || column <= 0 {
		return nil, false
	}
	for _, document := range documents {
		if filepath.ToSlash(document.Path) != path {
			continue
		}
		for blockID, span := range document.Spans {
			if !span.Contains(line) {
				continue
			}
			location := path + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(column)
			regularExpressions, err := assemblyline.TypeScriptRegularExpressionLiterals(
				document.Source, strings.HasSuffix(strings.ToLower(document.Path), ".tsx"),
			)
			if err != nil {
				return nil, false
			}
			return &directCodingStageDiagnostic{
				BlockID: blockID, DeclarationLine: line - span.StartLine + 1,
				DeclarationColumn: column, DocumentPath: path,
				DocumentLine: line, DocumentColumn: column,
				DocumentBlockStartLine: span.StartLine, DocumentBlockEndLine: span.EndLine,
				Message: location + "\n" + trimForBudget(output, 5000), Output: output,
				AuthorizedRegexLiterals: regularExpressions,
			}, true
		}
	}
	return nil, false
}
