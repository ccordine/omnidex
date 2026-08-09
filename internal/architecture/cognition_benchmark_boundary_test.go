package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const omnidexModulePath = "github.com/gryph/omnidex/"

var cognitionBenchmarkImports = []string{
	omnidexModulePath + "internal/cognitiongauntlet",
	omnidexModulePath + "internal/labyrinth",
}

func TestProductionCannotImportCognitionBenchmarkPackages(t *testing.T) {
	t.Parallel()
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	productionRoots := []string{
		filepath.Join(repositoryRoot, "internal"),
		filepath.Join(repositoryRoot, "cmd"),
	}
	for _, root := range productionRoots {
		violations, err := forbiddenGoImports(root, cognitionBenchmarkImports)
		if err != nil {
			t.Fatalf("scan production imports under %s: %v", root, err)
		}
		for _, violation := range violations {
			t.Errorf("production source imports benchmark-only package: %s", violation)
		}
	}
}

func TestProductionCognitionContainsNoLabyrinthVocabulary(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "cognition"))
	violations, err := forbiddenCognitionVocabulary(root)
	if err != nil {
		t.Fatalf("scan cognition vocabulary: %v", err)
	}
	for _, violation := range violations {
		t.Errorf("production cognition contains benchmark vocabulary: %s", violation)
	}
}

func TestCognitionBoundaryScannerRejectsForbiddenDirectionsAndAllowsBenchmarkConsumer(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	production := filepath.Join(root, "internal")
	worker := filepath.Join(production, "worker")
	cognition := filepath.Join(production, "cognition")
	benchmark := filepath.Join(root, "internal", "cognitiongauntlet")
	for _, directory := range []string{worker, cognition, benchmark} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeArchitectureFixture(t, filepath.Join(worker, "worker.go"), `package worker
import _ "github.com/gryph/omnidex/internal/labyrinth"
`)
	writeArchitectureFixture(t, filepath.Join(cognition, "cognition.go"), `package cognition
import _ "github.com/gryph/omnidex/internal/cognitiongauntlet"
`)
	writeArchitectureFixture(t, filepath.Join(benchmark, "benchmark.go"), `package cognitiongauntlet
import _ "github.com/gryph/omnidex/internal/cognition"
`)
	violations, err := forbiddenGoImports(production, cognitionBenchmarkImports)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 ||
		!containsArchitectureViolation(violations, "internal/labyrinth") ||
		!containsArchitectureViolation(violations, "internal/cognitiongauntlet") {
		t.Fatalf("forbidden production import violations=%v", violations)
	}
	for _, violation := range violations {
		if strings.Contains(violation, "benchmark.go") {
			t.Fatalf("benchmark consumer was forbidden from importing cognition: %v", violations)
		}
	}
}

func containsArchitectureViolation(violations []string, expected string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, expected) {
			return true
		}
	}
	return false
}

func TestCognitionVocabularyScannerSplitsIdentifiersAndText(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeArchitectureFixture(t, filepath.Join(root, "fixture.go"), `package cognition
type Key struct{}
const MazeRoomTreasure = "locked_doors shortest_path hidden_labels"
const mapKey = "domain-neutral"
`)
	violations, err := forbiddenCognitionVocabulary(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 7 {
		t.Fatalf("vocabulary violations=%v", violations)
	}
	for _, violation := range violations {
		if strings.Contains(violation, "mapKey") {
			t.Fatalf("generic key vocabulary was rejected: %v", violations)
		}
	}
}

func forbiddenGoImports(root string, forbidden []string) ([]string, error) {
	violations := []string{}
	err := walkProductionGo(root, func(path string, raw []byte) error {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, raw, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, imported := range parsed.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return fmt.Errorf("decode import in %s: %w", path, err)
			}
			for _, boundary := range forbidden {
				if value == boundary || strings.HasPrefix(value, boundary+"/") {
					violations = append(violations, path+" imports "+value)
				}
			}
		}
		return nil
	})
	return violations, err
}

func forbiddenCognitionVocabulary(root string) ([]string, error) {
	violations := []string{}
	err := walkProductionGo(root, func(path string, raw []byte) error {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, raw, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		seen := make(map[string]struct{})
		record := func(value string) {
			for _, label := range cognitionVocabularyLabels(value) {
				if _, duplicate := seen[label]; !duplicate {
					violations = append(violations, path+" contains "+strconv.Quote(label))
					seen[label] = struct{}{}
				}
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.Ident:
				record(value.Name)
			case *ast.BasicLit:
				if value.Kind == token.STRING {
					if decoded, decodeErr := strconv.Unquote(value.Value); decodeErr == nil {
						record(decoded)
					}
				}
			case *ast.TypeSpec:
				if value.Name.IsExported() && (value.Name.Name == "Key" || value.Name.Name == "Keys") {
					record("exported_key_domain_type")
				}
			}
			return true
		})
		return nil
	})
	return violations, err
}

func cognitionVocabularyLabels(value string) []string {
	words := sourceWords(value)
	alwaysForbidden := map[string]struct{}{
		"maze": {}, "mazes": {}, "labyrinth": {}, "labyrinths": {},
		"door": {}, "doors": {}, "room": {}, "rooms": {}, "treasure": {},
		"roguelike": {}, "inventory": {}, "combat": {}, "oracle": {}, "seed": {},
	}
	labels := []string{}
	for _, word := range words {
		if _, forbidden := alwaysForbidden[word]; forbidden {
			labels = append(labels, word)
		}
	}
	for _, phrase := range [][]string{
		{"shortest", "path"}, {"solution", "path"},
		{"benchmark", "score"}, {"hidden", "label"}, {"hidden", "labels"},
	} {
		if containsWordSequence(words, phrase) {
			labels = append(labels, strings.Join(phrase, "_"))
		}
	}
	if containsWordSequence(words, []string{"exported", "key", "domain", "type"}) {
		labels = append(labels, "exported_key_domain_type")
	}
	return labels
}

func containsWordSequence(words, sequence []string) bool {
	for start := 0; start+len(sequence) <= len(words); start++ {
		if strings.Join(words[start:start+len(sequence)], "\x00") == strings.Join(sequence, "\x00") {
			return true
		}
	}
	return false
}

func walkProductionGo(root string, inspect func(string, []byte) error) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && filepath.Dir(path) == root &&
				(entry.Name() == "cognitiongauntlet" || entry.Name() == "labyrinth") {
				return filepath.SkipDir
			}
			if path != root && (entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return inspect(path, raw)
	})
}

func sourceWords(source string) []string {
	words := []string{}
	current := []rune{}
	flush := func() {
		if len(current) != 0 {
			words = append(words, strings.ToLower(string(current)))
			current = current[:0]
		}
	}
	for _, character := range source {
		if !unicode.IsLetter(character) {
			flush()
			continue
		}
		if len(current) != 0 && unicode.IsUpper(character) && unicode.IsLower(current[len(current)-1]) {
			flush()
		}
		current = append(current, character)
	}
	flush()
	return words
}

func writeArchitectureFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
