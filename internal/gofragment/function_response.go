package gofragment

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"strings"

	"github.com/gryph/omnidex/internal/sourcebodyresponse"
)

// ExtractNewFunctionBodyResponse tests proposals in response order and retains
// the first body passing the parser and scope checks. Other declarations and
// examples grant no authority. If every body needs correction, the first body
// remains available to the ordinary exact-span validator. Raw proposals remain
// in the provider evidence; alternatives never enlarge the accepted task.
func ExtractNewFunctionBodyResponse(signature string, permittedSymbols []string, raw string) (string, error) {
	compiled, err := CompileNewFunctionSignature(signature)
	if err != nil {
		return "", err
	}
	candidates, err := sourcebodyresponse.ExtractCandidates(raw, maxGoRawSourceResponseBytes)
	if err != nil {
		return "", fmt.Errorf("Go source-body extraction: %w", err)
	}
	var matched, anonymous []string
	for _, candidate := range candidates {
		bodies, sole, err := goResponseDeclarationBodies(candidate.Source, compiled.Name)
		if err != nil {
			return "", err
		}
		if len(bodies) > 0 {
			matched = append(matched, bodies...)
		} else if sole != "" {
			anonymous = append(anonymous, sole)
		} else {
			assembled := compiled.Canonical + " {\n" + candidate.Source + "\n}"
			if _, err := parseOneFunction(assembled, false); err == nil || len(candidates) == 1 {
				anonymous = append(anonymous, candidate.Source)
			}
		}
	}
	if len(matched) == 0 {
		matched = anonymous
	}
	first := ""
	var normalizationErr error
	seen := make(map[string]bool)
	for _, proposal := range matched {
		body, err := normalizeGoSourceBody(proposal)
		if err != nil {
			normalizationErr = err
			continue
		}
		if seen[body] {
			continue
		}
		seen[body] = true
		if first == "" {
			first = body
		}
		if _, err := ParseNewFunction(compiled.Canonical, permittedSymbols, compiled.Canonical+" {\n"+body+"\n}"); err == nil {
			return body, nil
		}
	}
	if first != "" {
		return first, nil
	}
	if normalizationErr != nil {
		return "", normalizationErr
	}
	return "", fmt.Errorf("Go response contains no implementation body for the requested declaration")
}

func goResponseDeclarationBodies(source, expectedName string) ([]string, string, error) {
	fset := token.NewFileSet()
	var lexer scanner.Scanner
	lexer.Init(fset.AddFile("", fset.Base(), len(source)), []byte(source), nil, 0)
	_, first, _ := lexer.Scan()
	prefix := goFragmentFilePrefix
	if first == token.PACKAGE {
		prefix = ""
	}
	file, err := parser.ParseFile(fset, "", prefix+source, parser.AllErrors)
	if err != nil || file == nil {
		return nil, "", nil
	}
	var matched []string
	sole := ""
	functions := 0
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		start := fset.PositionFor(function.Body.Lbrace, false).Offset - len(prefix) + 1
		end := fset.PositionFor(function.Body.Rbrace, false).Offset - len(prefix)
		if start < 0 || end < start || end > len(source) {
			return nil, "", fmt.Errorf("parsed Go response body range is invalid")
		}
		body := source[start:end]
		sole = body
		functions++
		if function.Recv == nil && function.Name.Name == expectedName {
			matched = append(matched, body)
		}
	}
	if functions != 1 {
		sole = ""
	}
	return matched, sole, nil
}

func normalizeGoSourceBody(raw string) (string, error) {
	body := strings.ReplaceAll(raw, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	if strings.ContainsRune(body, '\x00') {
		return "", fmt.Errorf("Go source-body response contains invalid bytes")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("Go source-body response is empty")
	}
	if len(body) > maxGoExtractedSourceBodyBytes {
		return "", fmt.Errorf(
			"Go source-body response exceeds %d bytes", maxGoExtractedSourceBodyBytes,
		)
	}
	return body, nil
}
