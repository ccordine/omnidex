package gofragment

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"strings"

	"github.com/gryph/omnidex/internal/sourcebodyresponse"
)

const (
	goFragmentFilePrefix          = "package fragment\n\n"
	maxGoRawSourceResponseBytes   = 16 * 1024 * 1024
	maxGoExtractedSourceBodyBytes = 32 * 1024
)

// BodySpanViolation proves that a successfully parsed Go declaration has one
// exact failed byte range inside the model-returned implementation body.
// Declaration composition and the semantic correction question remain owned
// by the calling source adapter.
type BodySpanViolation struct {
	StartByte int
	EndByte   int
	Cause     error
}

func (violation *BodySpanViolation) Error() string {
	if violation == nil || violation.Cause == nil {
		return "Go source-body violation is nil"
	}
	return violation.Cause.Error()
}

func (violation *BodySpanViolation) Unwrap() error {
	if violation == nil {
		return nil
	}
	return violation.Cause
}

type Contract struct {
	Signature        string
	Current          string
	PermittedSymbols []string
}

type NewFunctionSignature struct {
	Canonical string
	Name      string
	Source    string
	StartByte int
	EndByte   int
}

// RequireSelfContainedNewFunctionSignature rejects declaration shapes whose
// surrounding import/type authority cannot be mechanically assembled yet.
// The rejection happens before inference; a future import contract can widen
// this without exposing a filename or whole-file responsibility to a model.
func RequireSelfContainedNewFunctionSignature(signature string) error {
	compiled, err := CompileNewFunctionSignature(signature)
	if err != nil {
		return err
	}
	function, err := parseOneFunction(
		compiled.Canonical+` { panic("omnidex code-owned placeholder") }`, true,
	)
	if err != nil {
		return err
	}
	qualified := ""
	ast.Inspect(function.Type, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && qualified == "" {
			qualified = selector.Sel.Name
		}
		return true
	})
	if qualified != "" {
		return fmt.Errorf(
			"new Go function signature requires unresolved import/type authority for %q", qualified,
		)
	}
	return nil
}

// CompileNewFunctionSignature parses code-owned or exact user-authored Go
// syntax without granting a model authority to invent the declaration shape.
// New methods are deliberately unsupported until receiver placement has its
// own exact identity contract.
func CompileNewFunctionSignature(signature string) (NewFunctionSignature, error) {
	signature = strings.TrimSpace(signature)
	if signature == "" || strings.ContainsAny(signature, "\r\n") {
		return NewFunctionSignature{}, fmt.Errorf("new Go function signature must be one line")
	}
	function, err := parseOneFunction(signature+` { panic("omnidex code-owned placeholder") }`, true)
	if err != nil {
		return NewFunctionSignature{}, fmt.Errorf("parse new Go function signature: %w", err)
	}
	if function.Recv != nil {
		return NewFunctionSignature{}, fmt.Errorf("new Go method signatures are unsupported without receiver placement authority")
	}
	canonical, err := functionSignature(function)
	if err != nil {
		return NewFunctionSignature{}, err
	}
	return NewFunctionSignature{Canonical: canonical, Name: function.Name.Name}, nil
}

// ParseNewFunction validates one newly required declaration under a signature
// already compiled by code. Unlike ParseFunction, it has no current model-owned
// declaration and therefore cannot infer mutation or placement authority.
func ParseNewFunction(signature string, permittedSymbols []string, candidate string) (string, error) {
	compiled, err := CompileNewFunctionSignature(signature)
	if err != nil {
		return "", err
	}
	placeholder := compiled.Canonical + ` { panic("omnidex code-owned placeholder") }`
	contract := Contract{
		Signature: compiled.Canonical, Current: placeholder,
		PermittedSymbols: append([]string(nil), permittedSymbols...),
	}
	placeholderCanonical, err := ParseFunction(contract, placeholder)
	if err != nil {
		return "", fmt.Errorf("compile new Go function placeholder: %w", err)
	}
	parsed, err := ParseFunction(contract, candidate)
	if err != nil {
		return "", err
	}
	if parsed == placeholderCanonical {
		return "", fmt.Errorf("new Go function candidate retained the code-owned placeholder")
	}
	return parsed, nil
}

// ParseNewFunctionBody applies an ordinary model response inside the exact
// code-owned function declaration before the existing parser and scope checks
// run. The model is never asked to reproduce declaration structure.
func ParseNewFunctionBody(
	signature string,
	permittedSymbols []string,
	rawBody string,
) (string, error) {
	compiled, err := CompileNewFunctionSignature(signature)
	if err != nil {
		return "", err
	}
	body, err := ExtractNewFunctionBodyResponse(compiled.Canonical, rawBody)
	if err != nil {
		return "", err
	}
	assembled := compiled.Canonical + " {\n" + body + "\n}"
	parsed, err := ParseNewFunction(
		compiled.Canonical,
		permittedSymbols,
		assembled,
	)
	if err == nil {
		return parsed, nil
	}
	var undeclared *UndeclaredIdentifierViolation
	if !errors.As(err, &undeclared) {
		return "", err
	}
	bodyStart := len(compiled.Canonical + " {\n")
	bodyEnd := bodyStart + len(body)
	if undeclared.StartByte < bodyStart || undeclared.EndByte > bodyEnd ||
		undeclared.StartByte >= undeclared.EndByte {
		return "", err
	}
	return "", &BodySpanViolation{
		StartByte: undeclared.StartByte - bodyStart,
		EndByte:   undeclared.EndByte - bodyStart,
		Cause:     err,
	}
}

// ExtractNewFunctionBodyResponse tolerates ordinary presentation noise without
// making it part of the source-body contract. A direct body is retained. A
// unique complete declaration is reduced to the bytes between its braces, and
// the declaration supplied by the model is discarded before validation.
func ExtractNewFunctionBodyResponse(signature, raw string) (string, error) {
	compiled, err := CompileNewFunctionSignature(signature)
	if err != nil {
		return "", err
	}
	candidate, err := sourcebodyresponse.ExtractCandidate(
		raw, maxGoRawSourceResponseBytes,
	)
	if err != nil {
		return "", fmt.Errorf("Go source-body extraction: %w", err)
	}
	if body, declaration, extractErr := extractGoDeclarationBody(candidate.Source); extractErr != nil {
		return "", fmt.Errorf("extract Go declaration body: %w", extractErr)
	} else if declaration {
		return normalizeGoSourceBody(body)
	}
	if candidate.Fenced {
		assembled := compiled.Canonical + " {\n" + candidate.Source + "\n}"
		if _, err := parseOneFunction(assembled, false); err != nil {
			return "", fmt.Errorf(
				"fenced Go response contains neither one declaration nor one parseable implementation body: %w",
				err,
			)
		}
	}
	return normalizeGoSourceBody(candidate.Source)
}

func extractGoDeclarationBody(source string) (string, bool, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(
		fileSet, "", goFragmentFilePrefix+source, parser.AllErrors|parser.ParseComments,
	)
	if err != nil || file == nil || len(file.Decls) != 1 {
		return "", false, nil
	}
	function, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || function.Body == nil {
		return "", false, nil
	}
	start := fileSet.Position(function.Pos()).Offset - len(goFragmentFilePrefix)
	end := fileSet.Position(function.End()).Offset - len(goFragmentFilePrefix)
	if start != 0 || end != len(source) {
		return "", false, nil
	}
	bodyStart := fileSet.Position(function.Body.Lbrace).Offset - len(goFragmentFilePrefix) + 1
	bodyEnd := fileSet.Position(function.Body.Rbrace).Offset - len(goFragmentFilePrefix)
	if bodyStart < 1 || bodyEnd < bodyStart || bodyEnd > len(source) {
		return "", false, fmt.Errorf("parsed declaration body range is invalid")
	}
	return source[bodyStart:bodyEnd], true, nil
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

// ParseFunction is the sole parser and capability validator for a complete Go
// function assembled by code. Candidate comments are forbidden before the Go
// parser runs so //line and //go directives cannot affect diagnostics or code.
func ParseFunction(contract Contract, candidate string) (string, error) {
	if strings.TrimSpace(contract.Signature) == "" || strings.ContainsAny(contract.Signature, "\r\n") {
		return "", fmt.Errorf("Go function contract requires one signature line")
	}
	current, err := parseOneFunction(contract.Current, true)
	if err != nil {
		return "", fmt.Errorf("parse current Go declaration: %w", err)
	}
	parsed, err := parseOneFunction(candidate, candidate == contract.Current)
	if err != nil {
		return "", err
	}
	expectedSignature, err := functionSignature(current)
	if err != nil {
		return "", err
	}
	if expectedSignature != strings.TrimSpace(contract.Signature) {
		return "", fmt.Errorf("current Go declaration signature %q differs from contract %q", expectedSignature, contract.Signature)
	}
	actualSignature, err := functionSignature(parsed)
	if err != nil {
		return "", err
	}
	if actualSignature != expectedSignature {
		return "", fmt.Errorf("Go function changed its exact signature: received %q want %q", actualSignature, expectedSignature)
	}
	if err := validateIdentifiers(current, parsed, contract.PermittedSymbols); err != nil {
		return "", err
	}
	return formatNode(parsed)
}

func parseOneFunction(source string, allowComments bool) (*ast.FuncDecl, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("Go fragment must contain one raw declaration")
	}
	if !allowComments && containsComment(source) {
		return nil, fmt.Errorf("Go fragment comments and compiler directives are forbidden")
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "", goFragmentFilePrefix+source, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("parse Go fragment: %w", err)
	}
	if len(file.Decls) != 1 {
		return nil, fmt.Errorf("Go fragment must contain exactly one declaration")
	}
	function, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok || function.Body == nil {
		return nil, fmt.Errorf("Go fragment must contain exactly one function or method declaration")
	}
	return function, nil
}

func containsComment(source string) bool {
	fileSet := token.NewFileSet()
	file := fileSet.AddFile("", fileSet.Base(), len(source))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(source), nil, scanner.ScanComments)
	for {
		_, current, _ := lexer.Scan()
		if current == token.COMMENT {
			return true
		}
		if current == token.EOF {
			return false
		}
	}
}

func functionSignature(function *ast.FuncDecl) (string, error) {
	copy := *function
	copy.Doc = nil
	copy.Body = nil
	return formatNode(&copy)
}

func formatNode(node ast.Node) (string, error) {
	var output bytes.Buffer
	if err := format.Node(&output, token.NewFileSet(), node); err != nil {
		return "", fmt.Errorf("format Go declaration: %w", err)
	}
	return strings.TrimSpace(output.String()), nil
}
