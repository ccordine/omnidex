package worker

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/artifacts"
)

var implementationGoVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)

func validateImplementationCandidateContent(item artifacts.ImplementationWorkItem, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("candidate content is empty")
	}
	switch strings.ToLower(filepath.Ext(item.Path)) {
	case ".go":
		return validateGoImplementationCandidate(item, content)
	case ".json":
		var value any
		if err := json.Unmarshal([]byte(content), &value); err != nil {
			return fmt.Errorf("candidate JSON is invalid: %w", err)
		}
	}
	if filepath.ToSlash(filepath.Clean(item.Path)) == "go.mod" {
		return validateGoModuleCandidate(content)
	}
	return nil
}

func validateGoImplementationCandidate(item artifacts.ImplementationWorkItem, content string) error {
	file, err := parser.ParseFile(token.NewFileSet(), item.Path, content, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("candidate Go syntax is invalid: %w", err)
	}
	mainFunctions := 0
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv == nil && function.Name.Name == "main" {
			mainFunctions++
		}
	}
	if item.Discipline == artifacts.ImplementationDisciplineEntrypoint {
		if file.Name.Name != "main" || mainFunctions != 1 {
			return fmt.Errorf("entrypoint Go file must declare package main and exactly one func main")
		}
		return nil
	}
	if mainFunctions != 0 {
		return fmt.Errorf("%s Go file cannot declare a program entrypoint", item.Discipline)
	}
	if item.Discipline == artifacts.ImplementationDisciplineTest && !strings.HasSuffix(strings.ToLower(item.Path), "_test.go") {
		return fmt.Errorf("test discipline Go file must end with _test.go")
	}
	return nil
}

func validateGoModuleCandidate(content string) error {
	modulePaths := make([]string, 0, 1)
	goVersions := make([]string, 0, 1)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 || strings.HasPrefix(fields[0], "//") {
			continue
		}
		switch fields[0] {
		case "module":
			if len(fields) != 2 {
				return fmt.Errorf("go.mod module directive requires exactly one path")
			}
			modulePaths = append(modulePaths, fields[1])
		case "go":
			if len(fields) != 2 || !implementationGoVersionPattern.MatchString(fields[1]) {
				return fmt.Errorf("go.mod go directive requires one valid Go language version")
			}
			goVersions = append(goVersions, fields[1])
		}
	}
	if len(modulePaths) != 1 {
		return fmt.Errorf("go.mod candidate requires exactly one module directive; received %d", len(modulePaths))
	}
	if err := validateGoModulePath(modulePaths[0]); err != nil {
		return fmt.Errorf("go.mod module path %q is invalid: %w", modulePaths[0], err)
	}
	if implementationModulePlaceholderPattern.MatchString(modulePaths[0]) {
		return fmt.Errorf("go.mod module path %q is a placeholder", modulePaths[0])
	}
	if len(goVersions) != 1 {
		return fmt.Errorf("go.mod candidate requires exactly one go directive; received %d", len(goVersions))
	}
	return nil
}

func validateGoModulePath(modulePath string) error {
	if !utf8.ValidString(modulePath) {
		return fmt.Errorf("invalid UTF-8")
	}
	if modulePath == "" {
		return fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(modulePath, "/") || strings.HasSuffix(modulePath, "/") {
		return fmt.Errorf("path cannot begin or end with a slash")
	}
	if strings.Contains(modulePath, "//") {
		return fmt.Errorf("path contains an empty element")
	}
	for _, element := range strings.Split(modulePath, "/") {
		if element == "" || strings.HasPrefix(element, ".") || strings.HasSuffix(element, ".") || strings.Contains(element, "..") {
			return fmt.Errorf("invalid path element %q", element)
		}
		for _, character := range element {
			if character == '-' || character == '.' || character == '_' || character == '~' ||
				character >= '0' && character <= '9' ||
				character >= 'A' && character <= 'Z' ||
				character >= 'a' && character <= 'z' {
				continue
			}
			return fmt.Errorf("invalid character %q", character)
		}
	}
	return nil
}
