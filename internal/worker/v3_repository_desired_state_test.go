package worker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestDesiredRepositoryStateCreatesStagesVerifiesAppliesAndReindexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	packageID := desiredStatePackageID(t, analysis, "verification")
	graph, err := repositoryfacts.NewDesiredArtifactGraph(snapshot, analysis, "objective_add_declaration", []repositoryfacts.DesiredGoArtifact{{
		RequirementQuote:  "Add func Added() int returning 2.",
		PackageArtifactID: packageID,
		Signature:         "func Added() int",
		MustExist:         true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := compileDesiredRepositoryFileStates(
		graph,
		snapshot,
		analysis,
		map[string]string{graph.Artifacts[0].ID: "func Added() int { return 2 }"},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertDesiredStateAuthorityCounters(t, compiled, 1, 0)
	if len(compiled.States) != 1 || compiled.States[0].Path != "omni_added_artifact.go" || !compiled.States[0].Present {
		t.Fatalf("compiled desired states=%+v", compiled.States)
	}
	want := "package verification\n\nfunc Added() int {\n\treturn 2\n}\n"
	if string(compiled.States[0].Content) != want {
		t.Fatalf("assembled source=%q want %q", compiled.States[0].Content, want)
	}

	stage, err := changeapply.PlanFileStateTransitions(ctx, changeapply.FileStateInput{
		Snapshot: snapshot, Analysis: analysis, OwnerID: graph.ID, Desired: compiled.States,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	desiredStateGoTest(t, ctx, snapshot.Root, stage)
	result, err := stage.ApplyVerified(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Action != "create" || result.Files[0].Path != "omni_added_artifact.go" {
		t.Fatalf("apply result=%+v", result)
	}
	fresh, freshAnalysis := desiredStateReindex(t, ctx, snapshot.Root)
	assertDesiredStateInventoryDelta(t, snapshot, fresh, "omni_added_artifact.go", true)
	added := existingRepositoryVerificationSymbol(t, freshAnalysis, "Added")
	if desiredStateFilePath(t, fresh, added.FileID) != "omni_added_artifact.go" {
		t.Fatalf("reindexed Added file=%q", desiredStateFilePath(t, fresh, added.FileID))
	}
}

func TestDesiredRepositoryStateDeletesStagesVerifiesAppliesAndReindexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	snapshot, _ := existingRepositoryVerificationFixture(t)
	obsoletePath := filepath.Join(snapshot.Root, "obsolete.go")
	if err := os.WriteFile(obsoletePath, []byte("package verification\n\nfunc Obsolete() int { return 9 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	desiredStateGit(t, snapshot.Root, "add", "obsolete.go")
	desiredStateGit(t, snapshot.Root, "commit", "-m", "obsolete fixture")
	snapshot, analysis := desiredStateReindex(t, ctx, snapshot.Root)
	obsolete := existingRepositoryVerificationSymbol(t, analysis, "Obsolete")
	graph := desiredStateDeletionGraph(t, snapshot, analysis, "verification", obsolete)
	compiled, err := compileDesiredRepositoryFileStates(graph, snapshot, analysis, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertDesiredStateAuthorityCounters(t, compiled, 0, 1)
	if len(compiled.States) != 1 || compiled.States[0].Path != "obsolete.go" || compiled.States[0].Present {
		t.Fatalf("compiled desired states=%+v", compiled.States)
	}

	stage, err := changeapply.PlanFileStateTransitions(ctx, changeapply.FileStateInput{
		Snapshot: snapshot, Analysis: analysis, OwnerID: graph.ID, Desired: compiled.States,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	desiredStateGoTest(t, ctx, snapshot.Root, stage)
	result, err := stage.ApplyVerified(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Action != "delete" || result.Files[0].Path != "obsolete.go" {
		t.Fatalf("apply result=%+v", result)
	}
	fresh, freshAnalysis := desiredStateReindex(t, ctx, snapshot.Root)
	assertDesiredStateInventoryDelta(t, snapshot, fresh, "obsolete.go", false)
	for _, symbol := range freshAnalysis.Symbols {
		if symbol.Name == "Obsolete" {
			t.Fatalf("deleted declaration remained in fresh index: %+v", symbol)
		}
	}
}

func TestDesiredRepositoryDeletionRejectsRemainingReference(t *testing.T) {
	t.Parallel()
	snapshot, analysis := existingRepositoryVerificationFixture(t)
	first := existingRepositoryVerificationSymbol(t, analysis, "First")
	graph := desiredStateDeletionGraph(t, snapshot, analysis, "verification", first)
	compiled, err := compileDesiredRepositoryFileStates(graph, snapshot, analysis, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = changeapply.PlanFileStateTransitions(context.Background(), changeapply.FileStateInput{
		Snapshot: snapshot, Analysis: analysis, OwnerID: graph.ID, Desired: compiled.States,
	})
	if err == nil || (!strings.Contains(err.Error(), "remains referenced") &&
		!strings.Contains(err.Error(), "last indexed Go build member")) {
		t.Fatalf("remaining-reference error=%v", err)
	}
}

func TestDesiredRepositoryDeletionRejectsGeneratedSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	snapshot, _ := existingRepositoryVerificationFixture(t)
	generatedPath := filepath.Join(snapshot.Root, "fixture.generated.go")
	if err := os.WriteFile(generatedPath, []byte("// Code generated by fixture. DO NOT EDIT.\n\npackage verification\n\nfunc GeneratedValue() int { return 3 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	desiredStateGit(t, snapshot.Root, "add", "fixture.generated.go")
	desiredStateGit(t, snapshot.Root, "commit", "-m", "generated fixture")
	snapshot, analysis := desiredStateReindex(t, ctx, snapshot.Root)
	generated := existingRepositoryVerificationSymbol(t, analysis, "GeneratedValue")
	graph := desiredStateDeletionGraph(t, snapshot, analysis, "verification", generated)
	compiled, err := compileDesiredRepositoryFileStates(graph, snapshot, analysis, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = changeapply.PlanFileStateTransitions(ctx, changeapply.FileStateInput{
		Snapshot: snapshot, Analysis: analysis, OwnerID: graph.ID, Desired: compiled.States,
	})
	if err == nil || !strings.Contains(err.Error(), "generated, protected") {
		t.Fatalf("generated deletion error=%v", err)
	}
}

func desiredStateDeletionGraph(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	packageName string,
	symbol repositoryfacts.Symbol,
) repositoryfacts.DesiredArtifactGraph {
	t.Helper()
	graph, err := repositoryfacts.NewDesiredArtifactGraph(snapshot, analysis, "objective_remove_declaration", []repositoryfacts.DesiredGoArtifact{{
		RequirementQuote:  "Remove the obsolete declaration.",
		PackageArtifactID: desiredStatePackageID(t, analysis, packageName),
		MustExist:         false,
		ExistingSymbolIDs: []string{symbol.ID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func assertDesiredStateAuthorityCounters(t *testing.T, got desiredRepositoryCompileResult, created, deleted int) {
	t.Helper()
	if got.CreatedFiles != created || got.DeletedFiles != deleted || got.DeterministicOperations != created+deleted {
		t.Fatalf("desired-state authority counters=%+v", got)
	}
}

func desiredStatePackageID(t *testing.T, analysis repositoryfacts.Analysis, name string) string {
	t.Helper()
	for _, artifact := range analysis.Artifacts {
		if artifact.Kind == "go_package" && artifact.Detail["package_name"] == name {
			return artifact.ID
		}
	}
	t.Fatalf("missing Go package artifact %q", name)
	return ""
}

func desiredStateGoTest(
	t *testing.T,
	ctx context.Context,
	root string,
	stage *changeapply.StagedChange,
) {
	t.Helper()
	output, err := desiredStateGoTestProjected(ctx, root, stage)
	if err != nil {
		t.Fatalf("verify projected desired repository: %v\n%s", err, output)
	}
}

func desiredStateGoTestProjected(
	ctx context.Context,
	root string,
	stage *changeapply.StagedChange,
) ([]byte, error) {
	replacements := make(map[string]string)
	for _, state := range stage.ExpectedFiles() {
		target := filepath.Join(root, filepath.FromSlash(state.Path))
		if state.Present {
			replacements[target] = filepath.Join(stage.DeltaRoot(), filepath.FromSlash(state.Path))
		} else {
			replacements[target] = ""
		}
	}
	raw, err := json.Marshal(struct {
		Replace map[string]string `json:"Replace"`
	}{Replace: replacements})
	if err != nil {
		return nil, err
	}
	overlay, err := os.CreateTemp("", "omnidex-go-overlay-*.json")
	if err != nil {
		return nil, err
	}
	overlayPath := overlay.Name()
	defer os.Remove(overlayPath)
	if _, err := overlay.Write(raw); err != nil {
		_ = overlay.Close()
		return nil, err
	}
	if err := overlay.Close(); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, "go", "test", "-overlay", overlayPath, "./...")
	command.Dir = root
	return command.CombinedOutput()
}

func desiredStateReindex(t *testing.T, ctx context.Context, root string) (repositoryfacts.Snapshot, repositoryfacts.Analysis) {
	t.Helper()
	snapshot, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, analysis
}

func assertDesiredStateInventoryDelta(t *testing.T, before, after repositoryfacts.Snapshot, path string, present bool) {
	t.Helper()
	beforePaths := make(map[string]string, len(before.Files))
	for _, file := range before.Files {
		beforePaths[file.Path] = file.SHA256
	}
	afterPaths := make(map[string]string, len(after.Files))
	for _, file := range after.Files {
		afterPaths[file.Path] = file.SHA256
	}
	for existingPath, sha := range beforePaths {
		if existingPath == path && !present {
			continue
		}
		if afterPaths[existingPath] != sha {
			t.Fatalf("unrelated repository inventory changed at %q", existingPath)
		}
	}
	if _, exists := afterPaths[path]; exists != present {
		t.Fatalf("post-state path %q present=%t want %t", path, exists, present)
	}
	if len(afterPaths)-len(beforePaths) != map[bool]int{true: 1, false: -1}[present] {
		t.Fatalf("inventory sizes before=%d after=%d", len(beforePaths), len(afterPaths))
	}
}

func desiredStateFilePath(t *testing.T, snapshot repositoryfacts.Snapshot, fileID string) string {
	t.Helper()
	for _, file := range snapshot.Files {
		if file.ID == fileID {
			return file.Path
		}
	}
	t.Fatalf("missing indexed file %q", fileID)
	return ""
}

func desiredStateGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
