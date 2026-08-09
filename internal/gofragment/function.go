package gofragment

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"strings"
)

type Contract struct {
	Signature        string
	Current          string
	PermittedSymbols []string
}

// ParseFunction is the sole parser and capability validator for a model-owned
// Go function or method block. Candidate comments are forbidden before the Go
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
	if source == "" || strings.Contains(source, "```") {
		return nil, fmt.Errorf("Go fragment must contain one raw declaration")
	}
	if !allowComments && containsComment(source) {
		return nil, fmt.Errorf("Go fragment comments and compiler directives are forbidden")
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "", "package fragment\n\n"+source, parser.AllErrors)
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
