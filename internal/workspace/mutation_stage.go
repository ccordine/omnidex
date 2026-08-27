package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gryph/omnidex/internal/omni"
)

type StagedMutation struct {
	mu sync.Mutex

	source    Snapshot
	plan      MutationPlan
	deltaRoot string
	closed    bool
	applied   bool
}

// StageMutation creates one bounded changed-file delta. It never copies the
// complete workspace: source files enter the delta only when their transition
// requires a replace or delete preimage.
func StageMutation(
	ctx context.Context,
	source Snapshot,
	plan MutationPlan,
) (_ *StagedMutation, resultErr error) {
	if ctx == nil {
		return nil, fmt.Errorf("stage workspace mutation requires a context")
	}
	if err := plan.ValidateSource(source); err != nil {
		return nil, err
	}
	if err := source.VerifyExact(ctx); err != nil {
		return nil, fmt.Errorf("stage workspace mutation source: %w", err)
	}
	deltaRoot, err := os.MkdirTemp("", "omnidex-workspace-delta-*")
	if err != nil {
		return nil, fmt.Errorf("create workspace mutation delta: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			resultErr = joinWorkspaceCleanupError(resultErr, os.RemoveAll(deltaRoot))
		}
	}()
	entries := make(map[string]Entry, len(source.Entries))
	for _, entry := range source.Entries {
		entries[entry.Path] = entry
	}
	for _, transition := range plan.Files {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("stage workspace mutation: %w", err)
		}
		parent := filepath.Dir(filepath.Join(deltaRoot, filepath.FromSlash(transition.Path)))
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return nil, fmt.Errorf("create workspace delta parent for %q: %w", transition.Path, err)
		}
		if !transition.Source.Present {
			continue
		}
		entry, exists := entries[transition.Path]
		if !exists {
			return nil, fmt.Errorf("workspace delta source %q disappeared from its snapshot", transition.Path)
		}
		content, err := readExactMutationSource(ctx, source.Root, entry)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(deltaRoot, filepath.FromSlash(transition.Path))
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create workspace delta source %q: %w", transition.Path, err)
		}
		writeErr := writeExactDeltaFile(file, content, os.FileMode(entry.Mode))
		if writeErr != nil {
			return nil, fmt.Errorf("write workspace delta source %q: %w", transition.Path, writeErr)
		}
	}
	if _, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{
		Context: ctx, Workspace: deltaRoot, Patch: plan.Patch, DryRun: true,
	}); err != nil {
		return nil, fmt.Errorf("dry-run workspace mutation delta: %w", err)
	}
	if _, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{
		Context: ctx, Workspace: deltaRoot, Patch: plan.Patch,
	}); err != nil {
		return nil, fmt.Errorf("apply workspace mutation delta: %w", err)
	}
	stage := &StagedMutation{
		source: cloneSnapshot(source), plan: cloneMutationPlan(plan), deltaRoot: deltaRoot,
	}
	if err := stage.verifyExactDelta(ctx); err != nil {
		return nil, err
	}
	if err := source.VerifyExact(ctx); err != nil {
		return nil, fmt.Errorf("workspace source changed while its delta was staged: %w", err)
	}
	keep = true
	return stage, nil
}

func writeExactDeltaFile(file *os.File, content []byte, mode os.FileMode) error {
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (stage *StagedMutation) DeltaRoot() string {
	if stage == nil {
		return ""
	}
	return stage.deltaRoot
}

func (stage *StagedMutation) SourceSnapshot() Snapshot {
	if stage == nil {
		return Snapshot{}
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	return cloneSnapshot(stage.source)
}

func (stage *StagedMutation) Plan() MutationPlan {
	if stage == nil {
		return MutationPlan{}
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	return cloneMutationPlan(stage.plan)
}

func (stage *StagedMutation) ApplyVerified(ctx context.Context) (omni.PatchApplyResult, error) {
	if stage == nil {
		return omni.PatchApplyResult{}, fmt.Errorf("apply verified workspace mutation requires a stage")
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return omni.PatchApplyResult{}, fmt.Errorf("workspace mutation stage %q is closed", stage.plan.ID)
	}
	if stage.applied {
		return omni.PatchApplyResult{}, fmt.Errorf("workspace mutation stage %q was already applied", stage.plan.ID)
	}
	if err := stage.plan.ValidateSource(stage.source); err != nil {
		return omni.PatchApplyResult{}, err
	}
	if err := stage.source.VerifyExact(ctx); err != nil {
		return omni.PatchApplyResult{}, err
	}
	if err := stage.verifyExactDelta(ctx); err != nil {
		return omni.PatchApplyResult{}, err
	}
	result, err := omni.ApplyUnifiedPatch(omni.PatchApplyOptions{
		Context: ctx, Workspace: stage.source.Root, Patch: stage.plan.Patch,
	})
	if err != nil {
		return omni.PatchApplyResult{}, fmt.Errorf("apply verified workspace mutation: %w", err)
	}
	if err := validateMutationApplyResult(stage.plan.Files, result.Files); err != nil {
		return omni.PatchApplyResult{}, err
	}
	stage.applied = true
	return result, nil
}

func (stage *StagedMutation) Cleanup() error {
	if stage == nil {
		return nil
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return nil
	}
	if err := os.RemoveAll(stage.deltaRoot); err != nil {
		return fmt.Errorf("clean workspace mutation stage %q: %w", stage.plan.ID, err)
	}
	stage.closed = true
	return nil
}

func cloneSnapshot(source Snapshot) Snapshot {
	clone := source
	clone.Entries = make([]Entry, len(source.Entries))
	copy(clone.Entries, source.Entries)
	clone.Exclusions = make([]Exclusion, len(source.Exclusions))
	copy(clone.Exclusions, source.Exclusions)
	if source.Git != nil {
		binding := *source.Git
		clone.Git = &binding
	}
	return clone
}

func cloneMutationPlan(source MutationPlan) MutationPlan {
	clone := source
	clone.Files = append([]MutationFileTransition(nil), source.Files...)
	return clone
}

func joinWorkspaceCleanupError(primary, cleanup error) error {
	if primary == nil {
		return cleanup
	}
	if cleanup == nil {
		return primary
	}
	return fmt.Errorf("%v; cleanup: %w", primary, cleanup)
}
