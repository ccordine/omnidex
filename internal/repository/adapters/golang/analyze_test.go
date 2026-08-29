package golang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestAnalyzeProducesCompilerBackedSymbolsCallsImportsAndTests(t *testing.T) {
	root := newGoAnalysisRepository(t)
	writeGoAnalysisFile(t, root, "go.mod", "module example.test/legacy\n\ngo 1.24\n")
	writeGoAnalysisFile(t, root, "legacy.go", `package legacy

import "fmt"

type Item struct { Name string }

func Helper(item Item) string {
	return fmt.Sprintf("item:%s", item.Name)
}

func Run() string {
	return Helper(Item{Name: "sample"})
}
`)
	writeGoAnalysisFile(t, root, "legacy_test.go", `package legacy

import "testing"

func TestRun(t *testing.T) {
	if Run() == "" { t.Fatal("empty") }
}
`)
	runGoAnalysisGit(t, root, "add", "go.mod", "legacy.go", "legacy_test.go")
	runGoAnalysisGit(t, root, "commit", "-m", "initial")
	snapshot, err := repositoryfacts.BuildGitSnapshot(context.Background(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}

	analysis, err := Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if err := analysis.Validate(snapshot); err != nil {
		t.Fatalf("analysis.Validate: %v", err)
	}
	if !analysis.Complete {
		t.Fatalf("analysis unexpectedly incomplete: %#v", analysis.Diagnostics)
	}
	if signature := goAnalysisSymbolSignature(analysis, "example.test/legacy.Run"); signature != "func Run() string" {
		t.Fatalf("Run signature=%q", signature)
	}
	for _, name := range []string{
		"example.test/legacy.Item",
		"example.test/legacy.Helper",
		"example.test/legacy.Run",
		"example.test/legacy.TestRun",
	} {
		if !hasGoAnalysisSymbol(analysis, name) {
			t.Errorf("analysis omitted symbol %q: %#v", name, analysis.Symbols)
		}
	}
	if !hasGoAnalysisArtifact(analysis, "go_package", "fmt") || !hasGoAnalysisArtifact(analysis, "go_package", "testing") {
		t.Fatalf("analysis omitted imported package artifacts: %#v", analysis.Artifacts)
	}
	if !hasGoAnalysisEdge(analysis, "calls", "example.test/legacy.Run", "example.test/legacy.Helper") {
		t.Fatalf("analysis omitted direct call edge: %#v", analysis.Edges)
	}
	if !hasGoAnalysisEdge(analysis, "tests", "example.test/legacy.TestRun", "example.test/legacy.Run") {
		t.Fatalf("analysis omitted direct test edge: %#v", analysis.Edges)
	}
}

func TestAnalyzeRecordsCompilerFailuresInsteadOfPretendingGraphIsComplete(t *testing.T) {
	root := newGoAnalysisRepository(t)
	writeGoAnalysisFile(t, root, "go.mod", "module example.test/broken\n\ngo 1.24\n")
	writeGoAnalysisFile(t, root, "broken.go", "package broken\n\nfunc Broken() MissingType { return MissingType{} }\n")
	runGoAnalysisGit(t, root, "add", "go.mod", "broken.go")
	runGoAnalysisGit(t, root, "commit", "-m", "initial")
	snapshot, err := repositoryfacts.BuildGitSnapshot(context.Background(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}

	analysis, err := Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Analyze should preserve parser facts with diagnostics: %v", err)
	}
	if analysis.Complete || len(analysis.Diagnostics) == 0 {
		t.Fatalf("incomplete type graph was reported as complete: %#v", analysis)
	}
	if err := analysis.Validate(snapshot); err != nil {
		t.Fatalf("incomplete evidence-bearing analysis is invalid: %v", err)
	}
}

func TestAnalyzeRejectsUncoveredIndexedGoFiles(t *testing.T) {
	for _, fixture := range []struct {
		name  string
		files map[string]string
	}{
		{
			name: "build constrained source",
			files: map[string]string{
				"go.mod":      "module example.test/coverage\n\ngo 1.24\n",
				"main.go":     "package coverage\n\nfunc Included() {}\n",
				"excluded.go": "//go:build omnidex_never_enabled\n\npackage coverage\n\nfunc Excluded() {}\n",
			},
		},
		{
			name: "nested module source",
			files: map[string]string{
				"go.mod":           "module example.test/root\n\ngo 1.24\n",
				"root.go":          "package root\n\nfunc Root() {}\n",
				"nested/go.mod":    "module example.test/nested\n\ngo 1.24\n",
				"nested/nested.go": "package nested\n\nfunc Nested() {}\n",
			},
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			root := newGoAnalysisRepository(t)
			paths := make([]string, 0, len(fixture.files))
			for path, content := range fixture.files {
				writeGoAnalysisFile(t, root, path, content)
				paths = append(paths, path)
			}
			runGoAnalysisGit(t, root, append([]string{"add"}, paths...)...)
			runGoAnalysisGit(t, root, "commit", "-m", "coverage")
			snapshot, err := repositoryfacts.BuildGitSnapshot(
				context.Background(), root, repositoryfacts.SnapshotOptions{},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Analyze(context.Background(), snapshot); err == nil ||
				!strings.Contains(err.Error(), "did not cover") {
				t.Fatalf("uncovered source error=%v", err)
			}
		})
	}
}

func TestGoAdapterSourceDoesNotUseRegexSymbolDiscovery(t *testing.T) {
	raw, err := os.ReadFile("analyze.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{"regexp.MustCompile", "FindAllStringSubmatch", "goSymbolPattern"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Go repository adapter contains forbidden regex discovery %q", forbidden)
		}
	}
}

func TestGoAnalysisEnvironmentRejectsAmbientRoutingAndUsesSnapshotRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "exact-root")
	environment := goAnalysisEnvironment([]string{
		"HOME=/controlled-home", "GOENV=/attacker/goenv", "GOFLAGS=-tags=attacker",
		"GOWORK=/attacker/go.work", "GOPROXY=https://attacker.invalid",
		"GOSUMDB=attacker.invalid", "GOTOOLCHAIN=attacker", "PWD=/attacker/root",
	}, root, "off")
	want := map[string]string{
		"HOME": "/controlled-home", "GO111MODULE": "on", "GOENV": "off",
		"GOFLAGS": "-mod=readonly", "GOWORK": "off", "GOPROXY": "off",
		"GOSUMDB": "off", "GOTOOLCHAIN": "local", "GOVCS": "off", "PWD": root,
	}
	seen := make(map[string]string, len(environment))
	for _, item := range environment {
		key, value, found := strings.Cut(item, "=")
		if !found {
			t.Fatalf("environment entry has no value: %q", item)
		}
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("environment key %q is duplicated: %v", key, environment)
		}
		seen[key] = value
	}
	for key, value := range want {
		if seen[key] != value {
			t.Fatalf("%s=%q, want %q in %v", key, seen[key], value, environment)
		}
	}
	if seen["GOFLAGS"] == "-tags=attacker" || seen["GOWORK"] == "/attacker/go.work" {
		t.Fatalf("hostile ambient Go routing survived: %v", environment)
	}
}

func TestExactGoWorkComesOnlyFromSnapshotAuthority(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projection")
	snapshot := repositoryfacts.Snapshot{Root: root, Files: []repositoryfacts.File{{
		Path: "go.mod", Kind: repositoryfacts.EntryRegular,
	}}}
	if got := exactGoWork(snapshot); got != "off" {
		t.Fatalf("snapshot without go.work selected %q", got)
	}
	snapshot.Files = append(snapshot.Files, repositoryfacts.File{
		Path: "go.work", Kind: repositoryfacts.EntryRegular,
	})
	if got, want := exactGoWork(snapshot), filepath.Join(root, "go.work"); got != want {
		t.Fatalf("exactGoWork=%q, want %q", got, want)
	}
	snapshot.Files[1].Kind = repositoryfacts.EntrySymlink
	if got := exactGoWork(snapshot); got != "off" {
		t.Fatalf("symlink go.work selected as authority: %q", got)
	}
}

func newGoAnalysisRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for Go analysis tests")
	}
	root := t.TempDir()
	runGoAnalysisGit(t, root, "init")
	runGoAnalysisGit(t, root, "config", "user.email", "go-analysis@example.test")
	runGoAnalysisGit(t, root, "config", "user.name", "Go Analysis Test")
	return root
}

func writeGoAnalysisFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGoAnalysisGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}

func hasGoAnalysisSymbol(analysis repositoryfacts.Analysis, qualified string) bool {
	for _, symbol := range analysis.Symbols {
		if symbol.QualifiedName == qualified {
			return true
		}
	}
	return false
}

func goAnalysisSymbolSignature(analysis repositoryfacts.Analysis, qualified string) string {
	for _, symbol := range analysis.Symbols {
		if symbol.QualifiedName == qualified {
			return symbol.Signature
		}
	}
	return ""
}

func hasGoAnalysisArtifact(analysis repositoryfacts.Analysis, kind, name string) bool {
	for _, artifact := range analysis.Artifacts {
		if artifact.Kind == kind && artifact.Name == name {
			return true
		}
	}
	return false
}

func hasGoAnalysisEdge(analysis repositoryfacts.Analysis, kind, from, to string) bool {
	symbols := make(map[string]string, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol.QualifiedName
	}
	for _, edge := range analysis.Edges {
		if edge.Kind == kind && symbols[edge.FromID] == from && symbols[edge.ToID] == to {
			return true
		}
	}
	return false
}
