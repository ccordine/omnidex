package workspace_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/workspace"
)

func TestCaptureEmptyNonGitWorkspaceAndVerifyExact(t *testing.T) {
	root := t.TempDir()
	snapshot, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(snapshot.ID, "workspace_state_") || snapshot.WorkspaceID == "" || snapshot.Root != root ||
		snapshot.Git != nil || snapshot.Entries == nil || len(snapshot.Entries) != 0 ||
		snapshot.Exclusions == nil {
		t.Fatalf("empty workspace snapshot=%+v", snapshot)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.VerifyExact(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestCapturePlainWorkspaceExcludesProtectedAndSensitiveAuthority(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/value.go", "package value\n", 0o640)
	writeWorkspaceFile(t, root, ".env", "SECRET=forbidden\n", 0o600)
	writeWorkspaceFile(t, root, "nested/private.pem", "forbidden\n", 0o600)
	writeWorkspaceFile(t, root, ".omni/state", "forbidden\n", 0o600)
	writeWorkspaceFile(t, root, "nested/.git/config", "forbidden\n", 0o600)
	if err := os.Symlink("value.go", filepath.Join(root, "src", "value-link")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got := workspaceEntryPaths(snapshot); strings.Join(got, ",") != "src/value-link,src/value.go" {
		t.Fatalf("plain snapshot paths=%v", got)
	}
	if snapshot.Entries[0].Kind != workspace.EntrySymlink || snapshot.Entries[0].LinkTarget != "value.go" {
		t.Fatalf("safe symlink entry=%+v", snapshot.Entries[0])
	}
	for _, forbidden := range []string{".env", ".omni/state", "nested/.git/config", "nested/private.pem"} {
		if !workspaceSnapshotExcludes(snapshot, forbidden) {
			t.Fatalf("workspace snapshot omitted exclusion %q: %+v", forbidden, snapshot.Exclusions)
		}
	}
	if err := snapshot.VerifyExact(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceSnapshotDetectsIncludedDriftButIgnoresExcludedContentChanges(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "value.txt", "one\n", 0o600)
	writeWorkspaceFile(t, root, ".env", "SECRET=one\n", 0o600)
	snapshot, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, ".env", "SECRET=two\n", 0o600)
	if err := snapshot.VerifyExact(t.Context()); err != nil {
		t.Fatalf("excluded content changed workspace authority: %v", err)
	}
	writeWorkspaceFile(t, root, ".env.local", "SECRET=three\n", 0o600)
	if err := snapshot.VerifyExact(t.Context()); err == nil || !strings.Contains(err.Error(), "exclusion authority changed") {
		t.Fatalf("excluded inventory drift error=%v", err)
	}
	withAdditionalExclusion, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if withAdditionalExclusion.ID == snapshot.ID {
		t.Fatal("workspace state identity ignored excluded-inventory drift")
	}
	if err := os.Remove(filepath.Join(root, ".env.local")); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "value.txt", "two\n", 0o600)
	if err := snapshot.VerifyExact(t.Context()); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("included drift error=%v", err)
	}
}

func TestWorkspaceStateIdentitySeparatesGitBindingFromIncludedFileState(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "value.txt", "same\n", 0o600)
	workspaceGit(t, root, "init")
	workspaceGit(t, root, "config", "user.name", "Omnidex Test")
	workspaceGit(t, root, "config", "user.email", "omnidex@example.invalid")
	workspaceGit(t, root, "add", ".")
	workspaceGit(t, root, "commit", "-m", "fixture")
	before, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	workspaceGit(t, root, "commit", "--allow-empty", "-m", "binding changed")
	after, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if before.ID != after.ID {
		t.Fatalf("included state identity changed with Git-only metadata: before=%s after=%s", before.ID, after.ID)
	}
	if err := before.VerifyExact(t.Context()); err == nil || !strings.Contains(err.Error(), "Git binding changed") {
		t.Fatalf("Git-only binding drift error=%v", err)
	}
}

func TestWorkspaceSnapshotConvertsExactGitRepositoryFacts(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "go.mod", "module example.com/workspace\n\ngo 1.24\n", 0o600)
	writeWorkspaceFile(t, root, "value.go", "package workspace\n", 0o640)
	workspaceGit(t, root, "init")
	workspaceGit(t, root, "config", "user.name", "Omnidex Test")
	workspaceGit(t, root, "config", "user.email", "omnidex@example.invalid")
	workspaceGit(t, root, "add", ".")
	workspaceGit(t, root, "commit", "-m", "fixture")
	repositorySnapshot, err := repositoryfacts.BuildGitSnapshot(
		context.Background(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := workspace.FromRepositorySnapshot(repositorySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Git == nil || snapshot.Git.RepositorySnapshotID != repositorySnapshot.ID ||
		len(snapshot.Entries) != len(repositorySnapshot.Files) {
		t.Fatalf("Git workspace snapshot=%+v", snapshot)
	}
	for index, entry := range snapshot.Entries {
		file := repositorySnapshot.Files[index]
		if entry.Path != file.Path || entry.SHA256 != file.SHA256 ||
			entry.RepositoryFileID != file.ID {
			t.Fatalf("workspace entry=%+v repository file=%+v", entry, file)
		}
	}
	captured, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if captured.ID != snapshot.ID {
		t.Fatalf("captured Git workspace=%s converted=%s", captured.ID, snapshot.ID)
	}
}

func TestCaptureNeverAscendsIntoParentGitRepository(t *testing.T) {
	parent := t.TempDir()
	workspaceGit(t, parent, "init")
	workspaceGit(t, parent, "config", "user.name", "Omnidex Test")
	workspaceGit(t, parent, "config", "user.email", "omnidex@example.invalid")
	writeWorkspaceFile(t, parent, "README.md", "parent\n", 0o600)
	workspaceGit(t, parent, "add", "README.md")
	workspaceGit(t, parent, "commit", "-m", "parent")
	root := filepath.Join(parent, "invoked-cwd")
	writeWorkspaceFile(t, root, "local.txt", "local\n", 0o600)
	snapshot, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Git != nil || strings.Join(workspaceEntryPaths(snapshot), ",") != "local.txt" {
		t.Fatalf("capture ascended into parent Git authority: %+v", snapshot)
	}
}

func TestCaptureFailsForInvalidRootLocalGitAuthority(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Capture(t.Context(), root); err == nil || !strings.Contains(err.Error(), "Git") {
		t.Fatalf("invalid root-local Git error=%v", err)
	}
}

func TestCaptureLargeWorkspaceProducesMetadataOnlyInventory(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 3_100; index++ {
		writeWorkspaceFile(
			t, root, filepath.Join("large", "nested", fmt.Sprintf("file-%04d.txt", index)),
			"exact\n", 0o600,
		)
	}
	snapshot, err := workspace.Capture(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 3_100 || snapshot.Root != root || snapshot.Git != nil {
		t.Fatalf("large workspace snapshot entries=%d root=%q Git=%+v", len(snapshot.Entries), snapshot.Root, snapshot.Git)
	}
	if err := snapshot.VerifyExact(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func writeWorkspaceFile(t *testing.T, root, relative, content string, mode os.FileMode) {
	t.Helper()
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func workspaceEntryPaths(snapshot workspace.Snapshot) []string {
	paths := make([]string, len(snapshot.Entries))
	for index, entry := range snapshot.Entries {
		paths[index] = entry.Path
	}
	return paths
}

func workspaceSnapshotExcludes(snapshot workspace.Snapshot, path string) bool {
	for _, exclusion := range snapshot.Exclusions {
		if exclusion.Path == path || strings.HasPrefix(path, exclusion.Path+"/") {
			return true
		}
	}
	return false
}

func workspaceGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
