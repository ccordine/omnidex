package changeapply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestPlanFileStateTransitionsCreatesOneCodeOwnedAbsentPath(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	content := []byte("package changeapply\n\nfunc Added() int { return 2 }\n")
	stage, err := changeapply.PlanFileStateTransitions(context.Background(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot,
		Analysis: fixture.analysis,
		OwnerID:  "objective_create_added",
		Desired: []changeapply.DesiredFileState{{
			Path: "added.go", Present: true, Content: content, Mode: 0o644,
			PackageArtifactID: packageArtifactID(t, fixture.analysis, "changeapply"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })

	if !strings.Contains(stage.Patch(), "--- /dev/null\n+++ b/added.go") {
		t.Fatalf("creation patch does not encode exact absence-to-source transition:\n%s", stage.Patch())
	}
	assertFile(t, filepath.Join(stage.Workspace(), "added.go"), string(content), 0o644)
	if len(stage.ExpectedFiles()) != 1 || !stage.ExpectedFiles()[0].Present ||
		stage.ExpectedFiles()[0].Path != "added.go" {
		t.Fatalf("expected states=%+v", stage.ExpectedFiles())
	}
	result, err := stage.ApplyVerified(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Action != "create" || result.Files[0].Path != "added.go" {
		t.Fatalf("apply result=%+v", result)
	}
	assertFile(t, filepath.Join(fixture.root, "added.go"), string(content), 0o644)
}

func TestPlanFileStateTransitionsDeletesOneExactUnreferencedSourceFile(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":      {content: "module example.com/delete\n\ngo 1.24\n", mode: 0o600},
		"retained.go": {content: "package deleted\n\nfunc Retained() int { return 1 }\n", mode: 0o600},
		"obsolete.go": {content: "package deleted\n\nfunc Obsolete() int { return 2 }\n", mode: 0o640},
	})
	obsolete := fixture.file(t, "obsolete.go")
	stage, err := changeapply.PlanFileStateTransitions(context.Background(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot,
		Analysis: fixture.analysis,
		OwnerID:  "objective_remove_obsolete",
		Desired: []changeapply.DesiredFileState{{
			Path: "obsolete.go", Present: false,
			RemovedSymbolIDs: []string{fixture.symbol(t, "Obsolete").ID},
			Source: changeapply.ExactSourceFile{
				FileID: obsolete.ID, SHA256: obsolete.SHA256, Size: obsolete.Size, Mode: obsolete.Mode,
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })

	if !strings.Contains(stage.Patch(), "--- a/obsolete.go\n+++ /dev/null") {
		t.Fatalf("deletion patch does not encode exact source-to-absence transition:\n%s", stage.Patch())
	}
	if _, err := os.Stat(filepath.Join(stage.Workspace(), "obsolete.go")); !os.IsNotExist(err) {
		t.Fatalf("obsolete staged file still exists: %v", err)
	}
	states := stage.ExpectedFiles()
	if len(states) != 1 || states[0].Present || states[0].FileID != obsolete.ID || states[0].Path != obsolete.Path {
		t.Fatalf("expected states=%+v", states)
	}
	result, err := stage.ApplyVerified(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0].Action != "delete" || result.Files[0].Path != obsolete.Path {
		t.Fatalf("apply result=%+v", result)
	}
	if _, err := os.Stat(filepath.Join(fixture.root, obsolete.Path)); !os.IsNotExist(err) {
		t.Fatalf("obsolete authoritative file still exists: %v", err)
	}
	assertFile(t, filepath.Join(fixture.root, "retained.go"), "package deleted\n\nfunc Retained() int { return 1 }\n", 0o600)
}

func TestPlanFileStateTransitionsRejectsCollisionStaleProtectedAndMalformedState(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	first := fixture.file(t, "first.go")
	validSource := changeapply.ExactSourceFile{
		FileID: first.ID, SHA256: first.SHA256, Size: first.Size, Mode: first.Mode,
	}
	cases := []struct {
		name    string
		desired changeapply.DesiredFileState
		want    string
	}{
		{name: "creation collision", desired: changeapply.DesiredFileState{
			Path: "first.go", Present: true, Content: []byte("package changeapply\n"), Mode: 0o644,
			PackageArtifactID: packageArtifactID(t, fixture.analysis, "changeapply"),
		}, want: "already exists"},
		{name: "stale deletion hash", desired: changeapply.DesiredFileState{
			Path: "first.go", Present: false,
			Source: changeapply.ExactSourceFile{FileID: first.ID, SHA256: strings.Repeat("0", 64), Size: first.Size, Mode: first.Mode},
		}, want: "source authority"},
		{name: "generated creation", desired: changeapply.DesiredFileState{
			Path: "added.generated.go", Present: true, Content: []byte("package changeapply\n"), Mode: 0o644,
			PackageArtifactID: packageArtifactID(t, fixture.analysis, "changeapply"),
		}, want: "protected"},
		{name: "missing delete authority", desired: changeapply.DesiredFileState{
			Path: "first.go", Present: false,
		}, want: "source authority"},
		{name: "delete content", desired: changeapply.DesiredFileState{
			Path: "first.go", Present: false, Source: validSource, Content: []byte("forbidden"),
		}, want: "absent state"},
		{name: "create source authority", desired: changeapply.DesiredFileState{
			Path: "added.go", Present: true, Source: validSource, Content: []byte("package changeapply\n"), Mode: 0o644,
			PackageArtifactID: packageArtifactID(t, fixture.analysis, "changeapply"),
		}, want: "absent source"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := changeapply.PlanFileStateTransitions(context.Background(), changeapply.FileStateInput{
				Snapshot: fixture.snapshot, Analysis: fixture.analysis,
				OwnerID: "objective_state_test", Desired: []changeapply.DesiredFileState{test.desired},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want %q", err, test.want)
			}
		})
	}
}

func TestPlanFileStateTransitionsRejectsUntrackedDeletionBeforeStaging(t *testing.T) {
	t.Parallel()
	base := basicFixture(t)
	path := "untracked.go"
	if err := os.WriteFile(
		filepath.Join(base.root, path),
		[]byte("package changeapply\n\nfunc Untracked() int { return 1 }\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		t.Context(), base.root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	current := &fixture{root: base.root, snapshot: snapshot, analysis: analysis}
	file := current.file(t, path)
	symbol := current.symbol(t, "Untracked")
	_, err = changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: snapshot, Analysis: analysis, OwnerID: "desired_graph_untracked",
		Desired: []changeapply.DesiredFileState{{
			Path: path, Present: false,
			Source: changeapply.ExactSourceFile{
				FileID: file.ID, SHA256: file.SHA256, Size: file.Size, Mode: file.Mode,
			},
			RemovedSymbolIDs: []string{symbol.ID},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "untracked") {
		t.Fatalf("untracked deletion error=%v", err)
	}
}

func TestPlanFileStateTransitionsRejectsCanonicalGoGeneratedSource(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod": {content: "module example.com/generated\n\ngo 1.24\n", mode: 0o644},
		"zz_generated.go": {
			content: "// Code generated by fixture. DO NOT EDIT.\npackage generated\n\nfunc Generated() int { return 1 }\n",
			mode:    0o644,
		},
	})
	file := fixture.file(t, "zz_generated.go")
	_, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "desired_graph_generated",
		Desired: []changeapply.DesiredFileState{{
			Path: file.Path, Present: false,
			Source: changeapply.ExactSourceFile{
				FileID: file.ID, SHA256: file.SHA256, Size: file.Size, Mode: file.Mode,
			},
			RemovedSymbolIDs: []string{fixture.symbol(t, "Generated").ID},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "generated") {
		t.Fatalf("canonical generated deletion error=%v", err)
	}
}

func TestPlanFileStateTransitionsRejectsAuthoritativeDriftBeforeApply(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	stage, err := changeapply.PlanFileStateTransitions(context.Background(), changeapply.FileStateInput{
		Snapshot: fixture.snapshot, Analysis: fixture.analysis, OwnerID: "objective_create_drift",
		Desired: []changeapply.DesiredFileState{{
			Path: "added.go", Present: true,
			Content: []byte("package changeapply\n\nfunc Added() int { return 2 }\n"), Mode: 0o644,
			PackageArtifactID: packageArtifactID(t, fixture.analysis, "changeapply"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	if err := os.WriteFile(filepath.Join(fixture.root, "added.go"), []byte("collision\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.ApplyVerified(context.Background()); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("drift apply error=%v", err)
	}
	assertFile(t, filepath.Join(fixture.root, "added.go"), "collision\n", 0o644)
}

func packageArtifactID(t *testing.T, analysis repositoryfacts.Analysis, packageName string) string {
	t.Helper()
	for _, artifact := range analysis.Artifacts {
		if artifact.Kind == "go_package" && artifact.Detail["package_name"] == packageName {
			return artifact.ID
		}
	}
	t.Fatalf("missing Go package artifact %q", packageName)
	return ""
}

func TestDesiredFileStateDoesNotEnterRepositoryModelContracts(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range []string{"assemblyline", "specialistworkflow"} {
		entries, err := os.ReadDir(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, relative, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"DesiredFileState", "ExactSourceFile", "PlanFileStateTransitions"} {
				if strings.Contains(string(raw), forbidden) {
					t.Fatalf("model contract %s exposes code-owned file-state authority %q", entry.Name(), forbidden)
				}
			}
		}
	}
	_ = repositoryfacts.EntryRegular
}
