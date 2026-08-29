package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/omni"
)

func (stage *StagedMutation) VerifyExactDelta(ctx context.Context) error {
	if stage == nil {
		return fmt.Errorf("verify workspace mutation delta requires a stage")
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return fmt.Errorf("workspace mutation stage %q is closed", stage.plan.ID)
	}
	return stage.verifyExactDelta(ctx)
}

func (stage *StagedMutation) verifyExactDelta(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("verify workspace mutation delta requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify workspace mutation delta: %w", err)
	}
	info, err := os.Lstat(stage.deltaRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace mutation delta root is absent or invalid")
	}
	expected := map[string]bool{".": true}
	states := make(map[string]MutationFileState, len(stage.plan.Files))
	for _, transition := range stage.plan.Files {
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(transition.Path))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			expected[parent] = true
		}
		if transition.Expected.Present {
			expected[transition.Path] = false
			states[transition.Path] = transition.Expected
		}
	}
	seen := make(map[string]struct{}, len(expected))
	err = filepath.WalkDir(stage.deltaRoot, func(absolute string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(stage.deltaRoot, absolute)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		wantDirectory, exists := expected[relative]
		if !exists || entry.IsDir() != wantDirectory {
			return fmt.Errorf("workspace mutation delta contains unexpected inventory entry %q", relative)
		}
		seen[relative] = struct{}{}
		if wantDirectory {
			return nil
		}
		state := states[relative]
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != os.FileMode(state.Mode) ||
			info.Size() != state.Size {
			return fmt.Errorf("workspace mutation delta file %q was tampered after staging", relative)
		}
		content, err := os.ReadFile(absolute)
		if err != nil || digestMutationBytes(content) != state.SHA256 {
			return fmt.Errorf("workspace mutation delta file %q was tampered after staging", relative)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("workspace mutation delta exact inventory is incomplete")
	}
	return nil
}

func validateMutationApplyResult(
	transitions []MutationFileTransition,
	files []omni.PatchFileResult,
) error {
	if len(files) != len(transitions) {
		return fmt.Errorf("workspace patch result differs from exact transition count")
	}
	expected := make(map[string]string, len(transitions))
	for _, transition := range transitions {
		action := "update"
		if !transition.Source.Present {
			action = "create"
		} else if !transition.Expected.Present {
			action = "delete"
		}
		expected[transition.Path] = action
	}
	for _, file := range files {
		if expected[file.Path] != file.Action {
			return fmt.Errorf("workspace patch result contains unexpected action for %q", file.Path)
		}
		delete(expected, file.Path)
	}
	if len(expected) != 0 {
		return fmt.Errorf("workspace patch result omitted exact transitions")
	}
	return nil
}
