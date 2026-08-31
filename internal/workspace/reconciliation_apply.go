package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type reconciliationDirectoryBackup struct {
	path string
	mode os.FileMode
}

type reconciliationApplyJournal struct {
	removed     []ReconciliationFileTransition
	written     []string
	createdDirs []string
	removedDirs []reconciliationDirectoryBackup
}

func (stage *StagedReconciliation) ApplyVerified(
	ctx context.Context,
) (ReconciliationResult, error) {
	if stage == nil {
		return ReconciliationResult{}, fmt.Errorf("apply verified workspace reconciliation requires a stage")
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return ReconciliationResult{}, fmt.Errorf("workspace reconciliation stage %q is closed", stage.plan.ID)
	}
	if stage.applied {
		return ReconciliationResult{}, fmt.Errorf("workspace reconciliation stage %q was already applied", stage.plan.ID)
	}
	if ctx == nil {
		return ReconciliationResult{}, fmt.Errorf("apply verified workspace reconciliation requires a context")
	}
	if err := stage.plan.validateSource(stage.source); err != nil {
		return ReconciliationResult{}, err
	}
	if err := stage.source.VerifyExact(ctx); err != nil {
		return ReconciliationResult{}, fmt.Errorf("apply workspace reconciliation source: %w", err)
	}
	if err := stage.verifyExactStage(ctx); err != nil {
		return ReconciliationResult{}, err
	}
	postState, err := stage.applyTransitions(ctx)
	if err != nil {
		return ReconciliationResult{}, err
	}
	stage.applied = true
	paths := make([]string, len(stage.plan.Files))
	for index, transition := range stage.plan.Files {
		paths[index] = transition.Path
	}
	return ReconciliationResult{
		PlanID: stage.plan.ID, SourceStateID: stage.plan.SourceStateID,
		ExpectedStateID: stage.plan.ExpectedStateID, ChangedPaths: paths,
		Moves: append([]ReconciliationMove(nil), stage.plan.Moves...),
		Snapshot: cloneSnapshot(postState),
	}, nil
}

func (stage *StagedReconciliation) applyTransitions(ctx context.Context) (Snapshot, error) {
	journal := reconciliationApplyJournal{}
	fail := func(primary error) (Snapshot, error) {
		rollbackErr := stage.rollback(ctx, journal)
		if rollbackErr != nil {
			return Snapshot{}, fmt.Errorf("%v; rollback: %w", primary, rollbackErr)
		}
		return Snapshot{}, primary
	}
	removals := append([]ReconciliationFileTransition(nil), stage.plan.Files...)
	sort.Slice(removals, func(left, right int) bool {
		leftDepth := reconciliationPathDepth(removals[left].Path)
		rightDepth := reconciliationPathDepth(removals[right].Path)
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return removals[left].Path > removals[right].Path
	})
	for _, transition := range removals {
		if !transition.Source.Present {
			continue
		}
		if err := verifyReconciliationPathState(ctx, stage.source.Root, transition.Path, transition.Source); err != nil {
			return fail(err)
		}
		absolute := filepath.Join(stage.source.Root, filepath.FromSlash(transition.Path))
		if err := os.Remove(absolute); err != nil {
			return fail(fmt.Errorf("remove workspace source %q: %w", transition.Path, err))
		}
		journal.removed = append(journal.removed, transition)
	}
	writes := append([]ReconciliationFileTransition(nil), stage.plan.Files...)
	sort.Slice(writes, func(left, right int) bool {
		leftDepth := reconciliationPathDepth(writes[left].Path)
		rightDepth := reconciliationPathDepth(writes[right].Path)
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return writes[left].Path < writes[right].Path
	})
	for _, transition := range writes {
		if !transition.Expected.Present {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fail(fmt.Errorf("apply workspace reconciliation: %w", err))
		}
		created, err := ensureReconciliationParents(stage.source.Root, transition.Path)
		if err != nil {
			return fail(err)
		}
		journal.createdDirs = append(journal.createdDirs, created...)
		target := filepath.Join(stage.source.Root, filepath.FromSlash(transition.Path))
		if info, err := os.Lstat(target); err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fail(fmt.Errorf("workspace reconciliation target %q unexpectedly exists", transition.Path))
			}
			if err := os.Remove(target); err != nil {
				return fail(fmt.Errorf("remove blocking workspace directory %q: %w", transition.Path, err))
			}
			journal.removedDirs = append(journal.removedDirs, reconciliationDirectoryBackup{
				path: transition.Path, mode: info.Mode().Perm(),
			})
		} else if !os.IsNotExist(err) {
			return fail(fmt.Errorf("inspect workspace reconciliation target %q: %w", transition.Path, err))
		}
		staged := filepath.Join(stage.desiredRoot, filepath.FromSlash(transition.Path))
		if err := writeStagedFileToWorkspace(ctx, staged, target, transition.Expected); err != nil {
			return fail(fmt.Errorf("write reconciled workspace file %q: %w", transition.Path, err))
		}
		journal.written = append(journal.written, transition.Path)
	}
	postState, err := Capture(ctx, stage.source.Root, stage.plan.Paths)
	if err != nil {
		return fail(fmt.Errorf("capture reconciled workspace: %w", err))
	}
	if err := stage.plan.VerifyExpected(postState); err != nil {
		return fail(err)
	}
	return postState, nil
}

func (stage *StagedReconciliation) rollback(
	ctx context.Context,
	journal reconciliationApplyJournal,
) error {
	rollbackCtx := context.WithoutCancel(ctx)
	var rollbackErrors []error
	for index := len(journal.written) - 1; index >= 0; index-- {
		relative := journal.written[index]
		if err := os.Remove(filepath.Join(stage.source.Root, filepath.FromSlash(relative))); err != nil &&
			!os.IsNotExist(err) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("remove reconciled path %q: %w", relative, err))
		}
	}
	sort.Slice(journal.removedDirs, func(left, right int) bool {
		return reconciliationPathDepth(journal.removedDirs[left].path) <
			reconciliationPathDepth(journal.removedDirs[right].path)
	})
	for _, directory := range journal.removedDirs {
		absolute := filepath.Join(stage.source.Root, filepath.FromSlash(directory.path))
		if err := os.Mkdir(absolute, directory.mode); err != nil && !os.IsExist(err) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore workspace directory %q: %w", directory.path, err))
		}
	}
	sort.Slice(journal.removed, func(left, right int) bool {
		return reconciliationPathDepth(journal.removed[left].Path) <
			reconciliationPathDepth(journal.removed[right].Path)
	})
	for _, transition := range journal.removed {
		if _, err := ensureReconciliationParents(stage.source.Root, transition.Path); err != nil {
			rollbackErrors = append(rollbackErrors, err)
			continue
		}
		target := filepath.Join(stage.source.Root, filepath.FromSlash(transition.Path))
		switch transition.Source.Kind {
		case EntryRegular:
			staged := filepath.Join(stage.sourceRoot, filepath.FromSlash(transition.Path))
			if err := writeStagedFileToWorkspace(rollbackCtx, staged, target, transition.Source); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore workspace file %q: %w", transition.Path, err))
			}
		case EntrySymlink:
			if err := os.Symlink(transition.Source.LinkTarget, target); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore workspace symlink %q: %w", transition.Path, err))
			}
		case EntryDirectory:
			if err := os.Mkdir(target, os.FileMode(transition.Source.Mode)); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore workspace directory %q: %w", transition.Path, err))
			}
		default:
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore workspace path %q has invalid source kind", transition.Path))
		}
	}
	sort.Slice(journal.createdDirs, func(left, right int) bool {
		return reconciliationPathDepth(journal.createdDirs[left]) > reconciliationPathDepth(journal.createdDirs[right])
	})
	for _, directory := range journal.createdDirs {
		absolute := filepath.Join(stage.source.Root, filepath.FromSlash(directory))
		entries, err := os.ReadDir(absolute)
		if os.IsNotExist(err) || err == nil && len(entries) != 0 {
			continue
		}
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("inspect created workspace directory %q: %w", directory, err))
			continue
		}
		if err := os.Remove(absolute); err != nil && !os.IsNotExist(err) {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("remove created workspace directory %q: %w", directory, err))
		}
	}
	if err := stage.source.VerifyExact(rollbackCtx); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("verify workspace rollback: %w", err))
	}
	return errors.Join(rollbackErrors...)
}

func ensureReconciliationParents(root, relative string) ([]string, error) {
	current := root
	parts := strings.Split(relative, "/")
	created := make([]string, 0)
	for index, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return created, fmt.Errorf("create workspace parent for %q: %w", relative, err)
			}
			created = append(created, strings.Join(parts[:index+1], "/"))
			info, err = os.Lstat(current)
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return created, fmt.Errorf("workspace reconciliation path %q has invalid parent authority", relative)
		}
	}
	return created, nil
}

func writeStagedFileToWorkspace(
	ctx context.Context,
	staged string,
	target string,
	expected ReconciliationFileState,
) (resultErr error) {
	input, err := os.Open(staged)
	if err != nil {
		return err
	}
	defer func() { resultErr = joinWorkspaceCleanupError(resultErr, input.Close()) }()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(target)
		}
	}()
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hash), &contextReader{ctx: ctx, reader: input})
	if copyErr == nil {
		copyErr = output.Sync()
	}
	if copyErr == nil {
		copyErr = output.Chmod(os.FileMode(expected.Mode))
	}
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("staged bytes differ from expected file state")
	}
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || info.Size() != expected.Size ||
		info.Mode().Perm() != os.FileMode(expected.Mode) {
		return fmt.Errorf("written file differs from expected filesystem state")
	}
	keep = true
	return nil
}

func verifyReconciliationPathState(
	ctx context.Context,
	root string,
	relative string,
	expected ReconciliationFileState,
) error {
	if err := rejectReconciliationSymlinkParents(root, relative, !expected.Present); err != nil {
		return err
	}
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	before, err := os.Lstat(absolute)
	if !expected.Present {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect absent workspace path %q: %w", relative, err)
		}
		return fmt.Errorf("workspace path %q appeared after source capture", relative)
	}
	if err != nil || uint32(before.Mode().Perm()) != expected.Mode ||
		expected.Kind != EntryDirectory && before.Size() != expected.Size {
		return fmt.Errorf("workspace path %q differs from exact source authority", relative)
	}
	switch expected.Kind {
	case EntryRegular:
		if !before.Mode().IsRegular() {
			return fmt.Errorf("workspace path %q differs from exact regular-file authority", relative)
		}
		digest, err := hashFile(ctx, absolute)
		after, afterErr := os.Lstat(absolute)
		if err != nil || afterErr != nil || !os.SameFile(before, after) ||
			before.Mode() != after.Mode() || before.Size() != after.Size() ||
			!before.ModTime().Equal(after.ModTime()) || digest != expected.SHA256 {
			return fmt.Errorf("workspace path %q changed while its source state was verified", relative)
		}
	case EntrySymlink:
		if before.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("workspace path %q differs from exact symlink authority", relative)
		}
		target, err := os.Readlink(absolute)
		if err != nil || target != expected.LinkTarget ||
			digestBytes([]byte("symlink\x00"+target)) != expected.SHA256 {
			return fmt.Errorf("workspace symlink %q changed after source capture", relative)
		}
	case EntryDirectory:
		if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 ||
			expected.SHA256 != digestBytes([]byte("directory\x00")) {
			return fmt.Errorf("workspace path %q differs from exact directory authority", relative)
		}
	default:
		return fmt.Errorf("workspace path %q has unsupported exact source kind", relative)
	}
	return nil
}

func rejectReconciliationSymlinkParents(root, relative string, allowMissing bool) error {
	current := root
	parts := strings.Split(relative, "/")
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if allowMissing && os.IsNotExist(err) {
			return nil
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace reconciliation path %q has invalid parent authority", relative)
		}
	}
	return nil
}
