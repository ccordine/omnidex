package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestRepositoryGoVerificationBubblewrapStagedDeltaIntegration(t *testing.T) {
	if os.Getenv("OMNIDEX_REQUIRE_BWRAP_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_REQUIRE_BWRAP_INTEGRATION=1 to require the real bubblewrap production proof")
	}
	if runtime.GOOS != "linux" {
		t.Fatalf("real repository verification sandbox requires Linux, received %s", runtime.GOOS)
	}
	t.Setenv("GOMODCACHE", t.TempDir())
	snapshot, _ := desiredVerificationDeletionFixture(t)
	if err := os.Symlink("first.go", filepath.Join(snapshot.Root, "first-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(snapshot.Root, ".gitignore"), []byte("large/private.env\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(snapshot.Root, "large", "clean"), 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3_100; index++ {
		path := filepath.Join(snapshot.Root, "large", "clean", fmt.Sprintf("file-%04d.txt", index))
		if err := os.WriteFile(path, []byte("unchanged large source\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(snapshot.Root, "large", "private.env"), []byte("STAGED_SECRET=forbidden\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	desiredStateGit(t, snapshot.Root, "add", ".gitignore", "first-link", "large/clean")
	desiredStateGit(t, snapshot.Root, "commit", "-m", "staged projection fixture")
	snapshot, analysis := desiredStateReindex(t, t.Context(), snapshot.Root)
	firstSymbol := existingRepositoryVerificationSymbol(t, analysis, "First")
	firstFile := exactRepositorySnapshotFile(t, snapshot, firstSymbol.FileID)
	obsoleteSymbol := existingRepositoryVerificationSymbol(t, analysis, "ObsoleteForVerification")
	obsoleteFile := exactRepositorySnapshotFile(t, snapshot, obsoleteSymbol.FileID)
	stage, err := changeapply.PlanFileStateTransitions(t.Context(), changeapply.FileStateInput{
		Snapshot: snapshot,
		Analysis: analysis,
		OwnerID:  "desired_graph_" + strings.Repeat("9", 64),
		Desired: []changeapply.DesiredFileState{
			{
				Path: firstFile.Path, Present: true,
				Source: changeapply.ExactSourceFile{
					FileID: firstFile.ID, SHA256: firstFile.SHA256,
					Size: firstFile.Size, Mode: firstFile.Mode,
				},
				Content: []byte("package verification\n\nfunc First() int { return 1 + 0 }\n"),
				Mode:    firstFile.Mode,
			},
			{
				Path: "added.go", Present: true, Mode: 0o644,
				Content:           []byte("package verification\n\nfunc Added() int { return 3 }\n"),
				PackageArtifactID: desiredVerificationPackageID(t, analysis, "verification"),
			},
			{
				Path: obsoleteFile.Path, Present: false,
				Source: changeapply.ExactSourceFile{
					FileID: obsoleteFile.ID, SHA256: obsoleteFile.SHA256,
					Size: obsoleteFile.Size, Mode: obsoleteFile.Mode,
				},
				RemovedSymbolIDs: []string{obsoleteSymbol.ID},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	projection, err := newRepositoryStagedProjection(t.Context(), stage)
	if err != nil {
		t.Fatal(err)
	}
	mounts, err := repositoryWorkspaceProjectionMounts(
		projection,
		repositoryWorkspaceProjectionMountRoots{base: snapshot.Root, delta: stage.DeltaRoot()},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertStagedRepositoryProjectionMounts(t, mounts)
	result, err := executeRepositoryGoVerification(
		t.Context(), projection, repositoryGoTestCall(30),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !directCodingCommandSucceeded(result) || len(result.Evidence) != 1 {
		t.Fatalf("real staged sandbox result=%#v evidence=%#v", result.Output, result.Evidence)
	}
	if raw, err := os.ReadFile(filepath.Join(snapshot.Root, firstFile.Path)); err != nil ||
		!strings.Contains(string(raw), "return 1 }") {
		t.Fatalf("staged verification mutated source first.go: %q error=%v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(snapshot.Root, "added.go")); !os.IsNotExist(err) {
		t.Fatalf("staged verification created source added.go: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snapshot.Root, obsoleteFile.Path)); err != nil {
		t.Fatalf("staged verification deleted authoritative source: %v", err)
	}
}

func assertStagedRepositoryProjectionMounts(
	t *testing.T,
	mounts []repositoryWorkspaceProjectionMount,
) {
	t.Helper()
	found := map[string]repositoryWorkspaceProjectionMount{}
	for _, mount := range mounts {
		found[mount.Path] = mount
	}
	for _, path := range []string{"first.go", "added.go"} {
		if found[path].Source != repositoryWorkspaceProjectionDelta || found[path].Directory {
			t.Fatalf("staged projection mount %q=%+v", path, found[path])
		}
	}
	if mount := found["large/clean"]; mount.Source != repositoryWorkspaceProjectionBase || !mount.Directory {
		t.Fatalf("large unchanged subtree mount=%+v", mount)
	}
	if mount := found["first-link"]; mount.Source != repositoryWorkspaceProjectionSymlink ||
		mount.LinkTarget != "first.go" {
		t.Fatalf("unchanged symlink mount=%+v", mount)
	}
	for _, absent := range []string{"obsolete_verification.go", "large/private.env"} {
		if _, exists := found[absent]; exists {
			t.Fatalf("absent path %q entered staged projection mounts", absent)
		}
	}
}
