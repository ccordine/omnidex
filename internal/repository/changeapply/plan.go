package changeapply

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/gryph/omnidex/internal/omni"
)

func Plan(ctx context.Context, input Input) (_ *StagedChange, err error) {
	if ctx == nil {
		return nil, fmt.Errorf("repository change staging requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("repository change staging: %w", err)
	}
	if err := input.Snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("repository change staging snapshot: %w", err)
	}
	if err := input.Analysis.Validate(input.Snapshot); err != nil {
		return nil, fmt.Errorf("repository change staging analysis: %w", err)
	}
	if err := input.Contract.Validate(input.Snapshot, input.Analysis); err != nil {
		return nil, fmt.Errorf("repository change staging contract: %w", err)
	}
	if err := verifyAuthoritativeSnapshot(ctx, input.Snapshot.Root, input.Snapshot.ID); err != nil {
		return nil, fmt.Errorf("repository change staging authority: %w", err)
	}
	replacements, err := resolveReplacements(input)
	if err != nil {
		return nil, err
	}
	workspace, err := stageSnapshot(ctx, input.Snapshot)
	if err != nil {
		return nil, err
	}
	keepWorkspace := false
	defer func() {
		if !keepWorkspace {
			err = joinCleanupError(err, os.RemoveAll(workspace))
		}
	}()
	mutations, err := planMutations(workspace, input.Snapshot, replacements)
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
	changedFileIDs := make([]string, len(mutations))
	expectedFiles := make([]ExpectedFileState, len(mutations))
	for index, mutation := range mutations {
		changedFileIDs[index] = mutation.file.ID
		expectedFiles[index] = ExpectedFileState{
			FileID: mutation.file.ID,
			SHA256: digest(mutation.next),
			Size:   int64(len(mutation.next)),
		}
	}
	sort.Strings(changedFileIDs)
	sort.Slice(expectedFiles, func(left, right int) bool {
		return expectedFiles[left].FileID < expectedFiles[right].FileID
	})
	stage := &StagedChange{
		id:        stageIdentity(input.Snapshot.ID, input.Contract.ID, patchHash),
		workspace: workspace, authoritativeRoot: input.Snapshot.Root,
		expectedSnapshotID: input.Snapshot.ID, patch: patch, patchSHA256: patchHash,
		changedFileIDs: changedFileIDs,
		expectedFiles:  expectedFiles,
		stagedFiles:    stagedFileAuthorities(input.Snapshot, mutations),
	}
	keepWorkspace = true
	return stage, nil
}
