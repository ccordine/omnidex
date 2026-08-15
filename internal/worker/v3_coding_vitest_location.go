package worker

import (
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
) (*directCodingStageDiagnostic, bool) {
	for _, location := range receipt.Locations {
		path, valid := directCodingVitestStagePath(root, location.File)
		if !valid {
			continue
		}
		diagnostic, mapped := mapDirectCodingTypeScriptDocumentLocation(
			documents, path, location.Line, location.Column, receipt.Output,
		)
		if !mapped {
			continue
		}
		diagnostic.FailureClass = receipt.FailureClass
		diagnostic.ModelFeedback = directCodingTypeScriptTestModelFailure(receipt.Output)
		return diagnostic, true
	}
	return nil, false
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
			return &directCodingStageDiagnostic{
				BlockID: blockID, DeclarationLine: line - span.StartLine + 1,
				DeclarationColumn: column,
				Message:           location + "\n" + trimForBudget(output, 5000),
				Output:            output,
			}, true
		}
	}
	return nil, false
}
