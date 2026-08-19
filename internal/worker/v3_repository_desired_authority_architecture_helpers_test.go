package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

type architectureSource struct {
	path    string
	file    *ast.File
	imports []string
}

func desiredModelBoundSources(t *testing.T) []architectureSource {
	t.Helper()
	sources := productionArchitectureSources(t, filepath.Join("..", "assemblyline"))
	for _, source := range productionArchitectureSources(t, ".") {
		modelBound := false
		ast.Inspect(source.file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && (identifier.Name == "RenderPortableJob" || identifier.Name == "runDirectCodingSemanticCall") {
				modelBound = true
			}
			return true
		})
		if modelBound {
			sources = append(sources, source)
		}
	}
	return sources
}

func desiredStateSources(t *testing.T) []architectureSource {
	t.Helper()
	patterns := []string{
		"v3_repository_desired_*.go",
		"v3_existing_repository_desired_*.go",
		"v3_artifact_candidate_*.go",
		"v3_known_artifact_truth*.go",
		"v3_path_free_deletion_*.go",
	}
	var sources []architectureSource
	for _, pattern := range patterns {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if !strings.HasSuffix(path, "_test.go") {
				sources = append(sources, parseArchitectureSource(t, path))
			}
		}
	}
	return sources
}

func productionArchitectureSources(t *testing.T, directory string) []architectureSource {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var sources []architectureSource
	for _, path := range paths {
		if !strings.HasSuffix(path, "_test.go") {
			sources = append(sources, parseArchitectureSource(t, path))
		}
	}
	return sources
}

func parseArchitectureSource(t *testing.T, path string) architectureSource {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, imported := range parsed.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		imports = append(imports, value)
	}
	return architectureSource{path: path, file: parsed, imports: imports}
}

func assertNoPhysicalSchemaAuthority(t *testing.T, label string, value any) {
	t.Helper()
	forbiddenProperties := stringSet(
		"path", "paths", "filepath", "filename", "file", "files", "operation", "operations",
		"action", "actions", "command", "commands", "shell", "patch", "patches", "content",
		"contents", "workspace", "tree", "tool", "tools", "arguments", "completion", "plan",
	)
	forbiddenValues := stringSet("create", "delete", "write", "remove", "rename", "move",
		"create_file", "delete_file", "write_file", "rename_file", "move_file")
	var inspect func(any)
	inspect = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			if properties, ok := typed["properties"].(map[string]any); ok {
				for property := range properties {
					if _, forbidden := forbiddenProperties[normalizeAuthorityName(property)]; forbidden {
						t.Errorf("%s response schema exposes physical property %q", label, property)
					}
				}
			}
			for key, child := range typed {
				if key == "const" || key == "enum" {
					assertNoPhysicalSchemaValue(t, label, child, forbiddenValues)
				}
				inspect(child)
			}
		case []any:
			for _, child := range typed {
				inspect(child)
			}
		}
	}
	inspect(value)
}

func assertNoPhysicalSchemaValue(t *testing.T, label string, value any, forbidden map[string]struct{}) {
	t.Helper()
	switch typed := value.(type) {
	case string:
		if _, exists := forbidden[strings.ToLower(typed)]; exists {
			t.Errorf("%s response schema exposes physical operation value %q", label, typed)
		}
	case []string:
		for _, item := range typed {
			assertNoPhysicalSchemaValue(t, label, item, forbidden)
		}
	case []any:
		for _, item := range typed {
			assertNoPhysicalSchemaValue(t, label, item, forbidden)
		}
	}
}

func normalizeAuthorityName(value string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(value))
}

func forbiddenModelAuthorityIdentifier(value string) bool {
	words := authorityIdentifierWords(value)
	for _, forbidden := range [][]string{
		{"write", "file"}, {"create", "file"}, {"delete", "file"}, {"rename", "file"},
		{"move", "file"}, {"remove", "file"}, {"apply", "patch"}, {"shell", "command"},
		{"run", "shell"}, {"execute", "shell"}, {"whole", "file"}, {"file", "contents"},
		{"generated", "file"}, {"file", "generation", "input"}, {"filesystem", "operation"},
		{"file", "operation"}, {"mutation", "operation"}, {"artifact", "action"},
		{"tool", "call"}, {"tool", "schema"}, {"action", "schema"},
		{"cognition", "decision"}, {"agent", "runtime"}, {"universal", "agent"},
		{"universal", "runtime"},
	} {
		if containsAuthorityWordSequence(words, forbidden) {
			return true
		}
	}
	return false
}

func authorityIdentifierWords(value string) []string {
	runes := []rune(value)
	words := make([]string, 0, 4)
	current := make([]rune, 0, len(runes))
	flush := func() {
		if len(current) == 0 {
			return
		}
		words = append(words, strings.ToLower(string(current)))
		current = current[:0]
	}
	for index, currentRune := range runes {
		if !unicode.IsLetter(currentRune) && !unicode.IsDigit(currentRune) {
			flush()
			continue
		}
		if len(current) > 0 && unicode.IsUpper(currentRune) {
			previousRune := runes[index-1]
			nextIsLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if unicode.IsLower(previousRune) || unicode.IsDigit(previousRune) ||
				(unicode.IsUpper(previousRune) && nextIsLower) {
				flush()
			}
		}
		current = append(current, currentRune)
	}
	flush()
	return words
}

func containsAuthorityWordSequence(words, sequence []string) bool {
	if len(words) == 0 || len(sequence) == 0 {
		return false
	}
	compact := strings.Join(sequence, "")
	for index, word := range words {
		if word == compact {
			return true
		}
		if index+len(sequence) > len(words) {
			continue
		}
		matched := true
		for offset, expected := range sequence {
			if words[index+offset] != expected {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
