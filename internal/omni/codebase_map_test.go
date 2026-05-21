package omni

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCodebaseMapRecordsFilesManifestsAndHashes(t *testing.T) {
	workspace := t.TempDir()
	writeCodebaseTestFile(t, workspace, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	writeCodebaseTestFile(t, workspace, "internal/omni/llm_command.go", "package omni\n\nfunc RunStructuredCommandDecision() {}\n")
	writeCodebaseTestFile(t, workspace, "internal/omni/llm_command_test.go", "package omni\n\nfunc TestLoop(t *testing.T) {}\n")

	cm, err := BuildCodebaseMap(workspace, CodebaseMapConfig{MaxFiles: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(cm.Files) != 3 || len(cm.Manifests) != 1 {
		t.Fatalf("files=%d manifests=%d map=%#v", len(cm.Files), len(cm.Manifests), cm)
	}
	if !codebaseMapHasFile(cm, "internal/omni/llm_command.go") {
		t.Fatalf("map missing llm_command.go: %#v", cm.Files)
	}
	if len(cm.Symbols) == 0 || cm.Symbols[0].Name != "RunStructuredCommandDecision" {
		t.Fatalf("symbols = %#v", cm.Symbols)
	}
	if cm.Files[0].SHA256 == "" || cm.Files[0].SummaryGeneratedForHash == "" {
		t.Fatalf("file hashes missing: %#v", cm.Files[0])
	}
}

func TestCodebaseMapUpdateMarksChangedFileSummaryStale(t *testing.T) {
	workspace := t.TempDir()
	writeCodebaseTestFile(t, workspace, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	writeCodebaseTestFile(t, workspace, "internal/omni/policy.go", "package omni\n\nfunc PolicyOne() {}\n")
	initial, err := BuildCodebaseMap(workspace, CodebaseMapConfig{MaxFiles: 100})
	if err != nil {
		t.Fatal(err)
	}
	target := DefaultCodebaseMapPath(workspace)
	if err := WriteCodebaseMap(initial, target); err != nil {
		t.Fatal(err)
	}
	writeCodebaseTestFile(t, workspace, "internal/omni/policy.go", "package omni\n\nfunc PolicyTwo() {}\n")

	updated, err := UpdateCodebaseMap(workspace, target, CodebaseMapConfig{MaxFiles: 100})
	if err != nil {
		t.Fatal(err)
	}
	file := findCodebaseFile(updated, "internal/omni/policy.go")
	if file == nil || !file.Stale {
		t.Fatalf("changed file should be stale: %#v", file)
	}
}

func TestCodebaseMapRouteScopeDriftFindsRuntimeFiles(t *testing.T) {
	workspace := t.TempDir()
	writeCodebaseTestFile(t, workspace, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	for _, path := range []string{
		"internal/omni/llm_command.go",
		"internal/omni/worksite_survey.go",
		"internal/omni/recipe.go",
		"internal/omni/policy.go",
		"internal/omni/llm_command_test.go",
	} {
		writeCodebaseTestFile(t, workspace, path, "package omni\n\nfunc ExampleSymbol() {}\n")
	}
	cm, err := BuildCodebaseMap(workspace, CodebaseMapConfig{MaxFiles: 100})
	if err != nil {
		t.Fatal(err)
	}
	route := RouteTaskWithCodebaseMap(cm, "fix scope drift handling in structured command loop")
	for _, want := range []string{"internal/omni/llm_command.go", "internal/omni/worksite_survey.go", "internal/omni/policy.go"} {
		if !containsCodebaseString(route.LikelyFiles, want) {
			t.Fatalf("route missing %s: %#v", want, route)
		}
	}
	if len(route.VerificationCommands) == 0 {
		t.Fatalf("route missing verification commands: %#v", route)
	}
}

func TestStructuredCommandUserMessageIncludesCodebaseTaskRoute(t *testing.T) {
	workspace := t.TempDir()
	writeCodebaseTestFile(t, workspace, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	writeCodebaseTestFile(t, workspace, "internal/omni/llm_command.go", "package omni\n\nfunc Loop() {}\n")
	cm, err := BuildCodebaseMap(workspace, CodebaseMapConfig{MaxFiles: 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteCodebaseMap(cm, DefaultCodebaseMapPath(workspace)); err != nil {
		t.Fatal(err)
	}
	message := buildStructuredCommandUserMessage("fix loop recovery", nil, workspace)
	for _, want := range []string{`"task_route"`, "internal/omni/llm_command.go", `"likely_files"`} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q: %s", want, message)
		}
	}
}

func TestAppRunCodebaseMapBuildAndRoute(t *testing.T) {
	workspace := t.TempDir()
	writeCodebaseTestFile(t, workspace, "go.mod", "module example.com/demo\n\ngo 1.22\n")
	writeCodebaseTestFile(t, workspace, "internal/omni/worksite_survey.go", "package omni\n\nfunc BuildWorksiteSurvey() {}\n")
	out := &bytes.Buffer{}
	app := NewApp(strings.NewReader(""), out, &bytes.Buffer{})
	if err := app.Run([]string{"map", "build", "--workspace", workspace}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Wrote codebase map") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	out.Reset()
	if err := app.Run([]string{"map", "route", "--workspace", workspace, "scope drift"}); err != nil {
		t.Fatal(err)
	}
	var route TaskRoute
	if err := json.Unmarshal(out.Bytes(), &route); err != nil {
		t.Fatalf("route JSON: %v\n%s", err, out.String())
	}
	if !containsCodebaseString(route.LikelyFiles, "internal/omni/worksite_survey.go") {
		t.Fatalf("route missing worksite file: %#v", route)
	}
}

func writeCodebaseTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func codebaseMapHasFile(cm CodebaseMap, path string) bool {
	return findCodebaseFile(cm, path) != nil
}

func findCodebaseFile(cm CodebaseMap, path string) *FileSummary {
	for i := range cm.Files {
		if cm.Files[i].Path == path {
			return &cm.Files[i]
		}
	}
	return nil
}

func containsCodebaseString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
