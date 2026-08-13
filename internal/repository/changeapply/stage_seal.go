package changeapply

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/gryph/omnidex/internal/omni"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

type fileMutationPlanner func(string) ([]fileMutation, error)

func stageAndSealMutations(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	ownerID string,
	plan fileMutationPlanner,
) (_ *StagedChange, err error) {
	if plan == nil {
		return nil, fmt.Errorf("repository change staging requires one exact mutation planner")
	}
	workspace, err := stageSnapshot(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	keepWorkspace := false
	defer func() {
		if !keepWorkspace {
			err = joinCleanupError(err, os.RemoveAll(workspace))
		}
	}()
	mutations, err := plan(workspace)
	if err != nil {
		return nil, err
	}
	patch, err := buildUnifiedPatch(mutations)
	if err != nil {
		return nil, err
	}
	if _, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{
		Context: ctx, Workspace: workspace, Patch: patch, DryRun: true,
	}); err != nil {
		return nil, fmt.Errorf("dry-run staged repository patch: %w", err)
	}
	if _, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{
		Context: ctx, Workspace: workspace, Patch: patch,
	}); err != nil {
		return nil, fmt.Errorf("apply staged repository patch: %w", err)
	}
	if err := verifyStagedMutations(workspace, mutations); err != nil {
		return nil, err
	}
	patchHash := digest([]byte(patch))
	changedFileIDs, expectedFiles := exactExpectedFileStates(mutations)
	stage := &StagedChange{
		id: stageIdentity(snapshot.ID, ownerID, patchHash), workspace: workspace,
		authoritativeRoot: snapshot.Root, expectedSnapshotID: snapshot.ID,
		patch: patch, patchSHA256: patchHash, changedFileIDs: changedFileIDs,
		expectedFiles: expectedFiles, stagedFiles: stagedFileAuthorities(snapshot, mutations),
	}
	keepWorkspace = true
	return stage, nil
}

func exactExpectedFileStates(mutations []fileMutation) ([]string, []ExpectedFileState) {
	changedFileIDs := make([]string, len(mutations))
	expectedFiles := make([]ExpectedFileState, len(mutations))
	for index, mutation := range mutations {
		changedFileIDs[index] = mutation.file.ID
		expectedFiles[index] = ExpectedFileState{
			FileID: mutation.file.ID, Path: mutation.file.Path,
			Present: mutation.desiredPresent,
		}
		if mutation.desiredPresent {
			expectedFiles[index].SHA256 = digest(mutation.next)
			expectedFiles[index].Size = int64(len(mutation.next))
			expectedFiles[index].Mode = mutation.file.Mode
		}
	}
	sort.Strings(changedFileIDs)
	sort.Slice(expectedFiles, func(left, right int) bool {
		return expectedFiles[left].FileID < expectedFiles[right].FileID
	})
	return changedFileIDs, expectedFiles
}
