package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestRepositoryWorkspaceProjectionCompactsLargeExactDirectoryWithoutCopyingFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "large", "clean"), 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3_100; index++ {
		path := filepath.Join(root, "large", "clean", fmt.Sprintf("file-%04d.txt", index))
		if err := os.WriteFile(path, []byte("exact projected content\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("large/private.env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "large", "private.env"), []byte("must stay outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repositorySandboxGit(t, root, "init")
	repositorySandboxGit(t, root, "config", "user.name", "Omnidex Test")
	repositorySandboxGit(t, root, "config", "user.email", "omnidex@example.invalid")
	repositorySandboxGit(t, root, "add", ".gitignore", "large/clean")
	repositorySandboxGit(t, root, "commit", "-m", "large projection fixture")
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		context.Background(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := newRepositorySnapshotProjection(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	mountRoots := repositoryWorkspaceProjectionMountRoots{base: projection.source.Root}
	mounts, err := repositoryWorkspaceProjectionMounts(projection, mountRoots)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 {
		t.Fatalf("large projection mounts=%d want 2: %+v", len(mounts), mounts)
	}
	if mounts[0].Path != ".gitignore" || mounts[0].Directory ||
		mounts[1].Path != "large/clean" || !mounts[1].Directory {
		t.Fatalf("large projection mounts=%+v", mounts)
	}
	arguments, err := repositoryGoSandboxArguments(projection, mountRoots, 3, -1, 4, 5, 6)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, "\x00")
	if !strings.Contains(joined, "/proc/self/fd/3/large/clean\x00/workspace/large/clean") {
		t.Fatalf("large exact directory was not descriptor-mounted: %q", joined)
	}
	for _, forbidden := range []string{"private.env", "file-0000.txt", "file-3099.txt"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("large projection arguments contain unapproved or uncompressed path %q", forbidden)
		}
	}
}

func TestRepositoryWorkspaceProjectionRejectsBubblewrapArgumentOverflow(t *testing.T) {
	goArgs := []string{"test", "./..."}
	outerCount := 4 + len(goArgs)
	arguments := make([]string, maxRepositoryBubblewrapParsedArguments-outerCount)
	for index := range arguments {
		arguments[index] = "bounded"
	}
	if _, err := repositoryBubblewrapInvocation(arguments, 3, goArgs); err != nil {
		t.Fatalf("exact Bubblewrap argument limit was rejected: %v", err)
	}
	arguments = append(arguments, "overflow")
	_, err := repositoryBubblewrapInvocation(arguments, 3, goArgs)
	if err == nil || !strings.Contains(err.Error(), "Bubblewrap arguments") {
		t.Fatalf("argument overflow error=%v", err)
	}
}

func TestRepositoryWorkspaceProjectionPlanningRemainsAnchoredToOpenSource(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "source")
	if err := os.MkdirAll(filepath.Join(root, "clean"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "clean", "approved.txt"), []byte("approved\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repositorySandboxGit(t, root, "init")
	repositorySandboxGit(t, root, "config", "user.name", "Omnidex Test")
	repositorySandboxGit(t, root, "config", "user.email", "omnidex@example.invalid")
	repositorySandboxGit(t, root, "add", "clean/approved.txt")
	repositorySandboxGit(t, root, "commit", "-m", "descriptor fixture")
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		context.Background(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := newRepositorySnapshotProjection(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if err := os.Rename(root, filepath.Join(parent, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "clean"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "clean", "unapproved.txt"), []byte("wrong tree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mounts, err := repositoryWorkspaceProjectionMounts(
		projection,
		repositoryWorkspaceProjectionMountRoots{
			base: fmt.Sprintf("/proc/self/fd/%d", handle.Fd()),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].Path != "clean" || !mounts[0].Directory {
		t.Fatalf("descriptor-anchored projection mounts=%+v", mounts)
	}
}
