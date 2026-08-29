package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestWorkspaceStagedProjectionSupportsEmptyPlainSourceWithoutTreeCopy(t *testing.T) {
	root := t.TempDir()
	source, err := workspacefacts.Capture(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		context.Background(), source, "desired_graph_"+strings.Repeat("a", 64),
		[]workspacefacts.DesiredFileState{{
			Path: "cmd/app/main.go", Present: true,
			Content: []byte("package main\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspacefacts.StageMutation(context.Background(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	projection, err := newWorkspaceStagedProjection(context.Background(), stage)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.source.Entries) != 0 || len(projection.files) != 1 ||
		projection.files[0].Path != "cmd/app/main.go" ||
		projection.files[0].Source != repositoryWorkspaceProjectionDelta {
		t.Fatalf("plain staged projection=%+v source=%+v", projection.files, projection.source.Entries)
	}
	mounts, err := repositoryWorkspaceProjectionMounts(
		projection,
		repositoryWorkspaceProjectionMountRoots{base: root, delta: stage.DeltaRoot()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 || mounts[0].Path != "cmd" || !mounts[0].Directory ||
		mounts[0].Source != repositoryWorkspaceProjectionDelta {
		t.Fatalf("plain staged mounts=%+v", mounts)
	}
	if _, err := os.Lstat(filepath.Join(root, "cmd")); !os.IsNotExist(err) {
		t.Fatalf("staging copied candidate into authoritative root: %v", err)
	}
}

func TestRepositoryWorkspaceProjectionRejectsProtectedPathComponents(t *testing.T) {
	t.Parallel()
	_, projection, _, _ := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 0))
	for _, path := range []string{".git/config", "nested/.git/config", ".omni/state", "nested/.omni/state"} {
		candidate := projection
		file := projection.files[0]
		file.Path = path
		candidate.files = []repositoryWorkspaceProjectionFile{file}
		if err := candidate.validate(); err == nil || !strings.Contains(err.Error(), "protected authority") {
			t.Fatalf("protected projection path %q error=%v", path, err)
		}
	}
}

func TestRepositoryWorkspaceProjectionRejectsFileAncestorCollision(t *testing.T) {
	t.Parallel()
	_, projection, _, _ := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 0))
	ancestor := projection.files[0]
	ancestor.Path = "collision"
	descendant := projection.files[0]
	descendant.Path = "collision/file.go"
	projection.files = []repositoryWorkspaceProjectionFile{ancestor, descendant}
	if err := projection.validate(); err == nil || !strings.Contains(err.Error(), "descends through file") {
		t.Fatalf("projection ancestor collision error=%v", err)
	}
}

func TestRepositoryWorkspaceProjectionRejectsSymlinkIntoNestedProtectedAuthority(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"nested/.git/config", "nested/.omni/state"} {
		if err := validateRepositoryProjectionSymlink("link", target); err == nil ||
			!strings.Contains(err.Error(), "escapes exact projected authority") {
			t.Fatalf("protected symlink target %q error=%v", target, err)
		}
	}
}
