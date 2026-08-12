package codingobjective

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"

	"github.com/gryph/omnidex/internal/omni"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func expectedFileAuthority(states []changeapply.ExpectedFileState) []ExpectedFile {
	result := make([]ExpectedFile, len(states))
	for index, state := range states {
		result[index] = ExpectedFile{FileID: state.FileID, SHA256: state.SHA256, Size: state.Size}
	}
	return result
}

func validateApplyResult(
	root string,
	snapshot repositoryfacts.Snapshot,
	changedFileIDs []string,
	result omni.PatchApplyResult,
) error {
	if filepath.Clean(result.Workspace) != root || result.DryRun {
		return fmt.Errorf("apply result does not identify the authoritative non-dry-run workspace")
	}
	paths := snapshotPaths(snapshot)
	want := make([]string, len(changedFileIDs))
	for index, fileID := range changedFileIDs {
		path, exists := paths[fileID]
		if !exists {
			return fmt.Errorf("changed file %q is absent from snapshot authority", fileID)
		}
		want[index] = path
	}
	got := make([]string, len(result.Files))
	for index, file := range result.Files {
		if file.Action != "update" {
			return fmt.Errorf("apply result action for %q is %q, want update", file.Path, file.Action)
		}
		got[index] = filepath.ToSlash(filepath.Clean(file.Path))
	}
	sort.Strings(want)
	sort.Strings(got)
	if len(got) != len(want) {
		return fmt.Errorf("apply result contains %d files, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("apply result file %q does not match expected %q", got[index], want[index])
		}
	}
	return nil
}

func reconcileExpectedRepository(
	ctx context.Context,
	root string,
	snapshot repositoryfacts.Snapshot,
	expected []ExpectedFile,
) error {
	expectedByID := make(map[string]ExpectedFile, len(expected))
	for _, file := range expected {
		if _, duplicate := expectedByID[file.FileID]; duplicate {
			return fmt.Errorf("expected file authority duplicates %q", file.FileID)
		}
		expectedByID[file.FileID] = file
	}
	wantFiles := append([]repositoryfacts.File(nil), snapshot.Files...)
	for index := range wantFiles {
		state, changed := expectedByID[wantFiles[index].ID]
		if !changed {
			continue
		}
		wantFiles[index].SHA256 = state.SHA256
		wantFiles[index].Size = state.Size
		delete(expectedByID, state.FileID)
	}
	if len(expectedByID) != 0 {
		return fmt.Errorf("expected file authority is absent from prior snapshot")
	}
	after, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		return fmt.Errorf("rebuild authoritative repository snapshot: %w", err)
	}
	if after.Schema != snapshot.Schema || after.RepositoryID != snapshot.RepositoryID ||
		after.HeadCommit != snapshot.HeadCommit {
		return fmt.Errorf("authoritative repository identity changed across commit")
	}
	if !reflect.DeepEqual(after.Files, wantFiles) || !reflect.DeepEqual(after.Exclusions, snapshot.Exclusions) {
		return fmt.Errorf("authoritative repository differs from exact expected indexed filesystem post-state")
	}
	return nil
}

func snapshotPaths(snapshot repositoryfacts.Snapshot) map[string]string {
	paths := make(map[string]string, len(snapshot.Files))
	for _, file := range snapshot.Files {
		paths[file.ID] = file.Path
	}
	return paths
}
