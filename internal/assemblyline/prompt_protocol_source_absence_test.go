package assemblyline

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

func TestProductionModelRenderersContainNoResponsePacketProtocol(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"return only",
		"return exactly",
		"respond only",
		"respond exactly",
		"reply only",
		"reply exactly",
		"response schema",
		"response packet",
		"response format",
		"path-safe",
		"previous implementation",
		"previous body",
		"change only",
		"preserve everything",
		"preserve all",
		"do not repeat",
		"don't repeat",
		"surrounding declaration",
		"surrounding structure",
		"extra declaration",
		"no declarations",
		"ast node",
		"syntax tree",
		"code fence",
		"markdown fence",
		"without markdown",
		"raw code",
		"implementation body",
		"function body",
	}
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages["assemblyline"]
	if pkg == nil {
		t.Fatal("assemblyline production package was not parsed")
	}
	for filename, file := range pkg.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || !modelRendererFunction(function.Name.Name) {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, unquoteErr := strconv.Unquote(literal.Value)
				if unquoteErr != nil {
					t.Fatalf("%s %s contains an invalid string literal: %v", filename, function.Name.Name, unquoteErr)
				}
				lower := strings.ToLower(value)
				for _, phrase := range forbidden {
					if strings.Contains(lower, phrase) {
						t.Errorf(
							"%s %s contains forbidden model response protocol %q",
							filename, function.Name.Name, phrase,
						)
					}
				}
				if strings.Contains(lower, "answer with") &&
					function.Name.Name != "RenderOpaqueModelChoiceQuestion" {
					t.Errorf(
						"%s %s defines a textual answer protocol outside the opaque-choice renderer",
						filename, function.Name.Name,
					)
				}
				for _, tokenValue := range uppercaseControlTokens(value) {
					if tokenValue != ApplicationNoRuntimeRequirementCandidates {
						t.Errorf(
							"%s %s exposes forbidden control token %q",
							filename, function.Name.Name, tokenValue,
						)
					}
				}
				return true
			})
		}
	}
}

func modelRendererFunction(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "prompt") ||
		strings.Contains(lower, "modelinput") ||
		strings.Contains(lower, "modeltext")
}

func uppercaseControlTokens(value string) []string {
	tokens := []string{}
	for _, field := range strings.FieldsFunc(value, func(value rune) bool {
		return !(value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_')
	}) {
		if strings.HasPrefix(field, "NO_") || strings.HasPrefix(field, "NONE_") {
			tokens = append(tokens, field)
		}
	}
	return tokens
}
