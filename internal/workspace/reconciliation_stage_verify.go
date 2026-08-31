package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func (stage *StagedReconciliation) VerifyExactStage(ctx context.Context) error {
	if stage == nil {
		return fmt.Errorf("verify workspace reconciliation stage requires a stage")
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return fmt.Errorf("workspace reconciliation stage %q is closed", stage.plan.ID)
	}
	return stage.verifyExactStage(ctx)
}

func (stage *StagedReconciliation) verifyExactStage(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("verify workspace reconciliation stage requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify workspace reconciliation stage: %w", err)
	}
	info, err := os.Lstat(stage.root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace reconciliation stage root is absent or invalid")
	}
	expected := map[string]bool{".": true, "desired": true, "source": true}
	states := make(map[string]ReconciliationFileState, len(stage.plan.Files)*2)
	for _, transition := range stage.plan.Files {
		if transition.Expected.Present {
			addStageFileExpectation(expected, "desired/"+transition.Path)
			states["desired/"+transition.Path] = transition.Expected
		}
		if transition.Source.Present && transition.Source.Kind == EntryRegular {
			addStageFileExpectation(expected, "source/"+transition.Path)
			states["source/"+transition.Path] = transition.Source
		}
	}
	seen := make(map[string]struct{}, len(expected))
	err = filepath.WalkDir(stage.root, func(absolute string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(stage.root, absolute)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		wantDirectory, exists := expected[relative]
		if !exists || entry.IsDir() != wantDirectory {
			return fmt.Errorf("workspace reconciliation stage contains unexpected inventory entry %q", relative)
		}
		seen[relative] = struct{}{}
		if wantDirectory {
			return nil
		}
		state := states[relative]
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 ||
			info.Size() != state.Size {
			return fmt.Errorf("workspace reconciliation staged file %q was tampered", relative)
		}
		digest, err := hashFile(ctx, absolute)
		if err != nil || digest != state.SHA256 {
			return fmt.Errorf("workspace reconciliation staged file %q was tampered", relative)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("workspace reconciliation stage exact inventory is incomplete")
	}
	return nil
}

func addStageFileExpectation(expected map[string]bool, relative string) {
	expected[relative] = false
	for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative)));
		parent != ".";
		parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
		expected[parent] = true
	}
}
