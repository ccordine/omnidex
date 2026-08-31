package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
)

func (prepared *PreparedReconciliation) ApplyVerified(
	ctx context.Context,
) (ReconciliationResult, error) {
	if prepared == nil {
		return ReconciliationResult{}, fmt.Errorf("apply workspace reconciliation requires a prepared transaction")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.applied {
		return ReconciliationResult{}, fmt.Errorf("workspace reconciliation was already applied")
	}
	if ctx == nil {
		return ReconciliationResult{}, fmt.Errorf("apply workspace reconciliation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return ReconciliationResult{}, fmt.Errorf("apply workspace reconciliation: %w", err)
	}
	root, err := os.OpenRoot(prepared.root)
	if err != nil {
		return ReconciliationResult{}, fmt.Errorf("open authoritative workspace root: %w", err)
	}
	current, err := root.Stat(".")
	if err != nil || !current.IsDir() || !os.SameFile(prepared.rootInfo, current) {
		return ReconciliationResult{}, errors.Join(
			fmt.Errorf("authoritative workspace root changed before mutation"), root.Close(),
		)
	}
	result, applyErr := applyReconciliation(ctx, root, prepared.desired)
	closeErr := root.Close()
	if applyErr != nil {
		return result, errors.Join(applyErr, closeErr)
	}
	if closeErr != nil {
		result.Warnings = append(result.Warnings, fmt.Errorf(
			"close authoritative workspace root: %w", closeErr,
		).Error())
	}
	prepared.applied = true
	return result, nil
}

func applyReconciliation(
	ctx context.Context,
	root *os.Root,
	desired []DesiredFile,
) (ReconciliationResult, error) {
	result := ReconciliationResult{Changes: []Change{}}
	moves := make([]DesiredFile, 0)
	for _, state := range desired {
		if state.MoveFrom != "" {
			moves = append(moves, state)
		}
	}
	if err := applyMoves(ctx, root, moves, &result); err != nil {
		return result, err
	}
	for _, state := range desired {
		if state.MoveFrom != "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("apply workspace reconciliation: %w", err)
		}
		if state.Present {
			if state.PreserveExisting {
				if err := applyRequiredFile(ctx, root, state, &result); err != nil {
					return result, err
				}
				continue
			}
			if err := applyFile(ctx, root, state, &result); err != nil {
				return result, err
			}
			continue
		}
		if err := applyDeletion(root, state.Path, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func applyRequiredFile(
	ctx context.Context,
	root *os.Root,
	state DesiredFile,
	result *ReconciliationResult,
) error {
	missingParent, err := inspectWorkspaceParents(root, state.Path, false)
	if err != nil {
		return err
	}
	if !missingParent {
		info, err := root.Lstat(state.Path)
		if err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("required workspace file %q is not a regular file", state.Path)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect required workspace file %q: %w", state.Path, err)
		}
	}
	state.PreserveExisting = false
	return applyFile(ctx, root, state, result)
}

func applyFile(
	ctx context.Context,
	root *os.Root,
	state DesiredFile,
	result *ReconciliationResult,
) (resultErr error) {
	if _, err := inspectWorkspaceParents(root, state.Path, true); err != nil {
		return err
	}
	before, err := root.Lstat(state.Path)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect workspace file %q: %w", state.Path, err)
	}
	if existed && before.IsDir() {
		return fmt.Errorf("workspace file %q is blocked by a directory", state.Path)
	}
	if existed && !before.Mode().IsRegular() && before.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("workspace file %q is blocked by a special filesystem node", state.Path)
	}
	if existed && before.Mode().IsRegular() {
		file, matches, err := openRegularFileContentMatch(ctx, root, state.Path, before, state.Content)
		if err != nil {
			return err
		}
		if matches {
			if file == nil {
				changed, err := verifyPathFileMode(root, state.Path, before, state.Mode)
				if err != nil {
					return err
				}
				if changed {
					result.Changes = append(result.Changes, Change{Path: state.Path, Kind: ChangeReplace})
				}
				return nil
			} else {
			changed, err := verifyOpenFileMode(root, state.Path, file, before, state.Mode, result)
			if err != nil {
				return err
			}
			if changed {
				result.Changes = append(result.Changes, Change{Path: state.Path, Kind: ChangeReplace})
			}
			return nil
			}
		}
		closeWorkspaceHandle(result, state.Path, file)
	}
	temporary, temporaryInfo, err := writePreparedFile(
		ctx, root, path.Dir(state.Path), state.Content, state.Mode,
	)
	if err != nil {
		return fmt.Errorf("prepare workspace file %q: %w", state.Path, err)
	}
	keepTemporary := true
	defer func() {
		if keepTemporary {
			resultErr = errors.Join(resultErr, removeIfPresent(root, temporary))
		}
	}()
	if err := root.Rename(temporary, state.Path); err != nil {
		return fmt.Errorf("install workspace file %q: %w", state.Path, err)
	}
	keepTemporary = false
	kind := ChangeCreate
	if existed {
		kind = ChangeReplace
	}
	result.Changes = append(result.Changes, Change{Path: state.Path, Kind: kind})
	after, err := root.Lstat(state.Path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(temporaryInfo, after) ||
		after.Size() != int64(len(state.Content)) || after.Mode().Perm() != os.FileMode(state.Mode) {
		return fmt.Errorf("installed workspace file %q differs from desired bytes or permissions", state.Path)
	}
	return nil
}

func verifyOpenFileMode(
	root *os.Root,
	relative string,
	file *os.File,
	before os.FileInfo,
	mode uint32,
	result *ReconciliationResult,
) (bool, error) {
	if root == nil || file == nil || before == nil {
		return false, fmt.Errorf("verify workspace file mode requires exact open-file authority")
	}
	changed := before.Mode().Perm() != os.FileMode(mode)
	if changed {
		if err := file.Chmod(os.FileMode(mode)); err != nil {
			closeWorkspaceHandle(result, relative, file)
			return false, fmt.Errorf("set workspace file mode %q: %w", relative, err)
		}
	}
	fileInfo, fileErr := file.Stat()
	pathInfo, pathErr := root.Lstat(relative)
	closeWorkspaceHandle(result, relative, file)
	if fileErr != nil || pathErr != nil || !fileInfo.Mode().IsRegular() ||
		!os.SameFile(before, fileInfo) || !os.SameFile(before, pathInfo) ||
		fileInfo.Mode().Perm() != os.FileMode(mode) ||
		pathInfo.Mode().Perm() != os.FileMode(mode) {
		return changed, fmt.Errorf("workspace file %q differs from desired permissions", relative)
	}
	return changed, nil
}

func verifyPathFileExact(
	root *os.Root,
	relative string,
	before os.FileInfo,
	mode uint32,
) error {
	if root == nil || before == nil {
		return fmt.Errorf("verify workspace file requires exact path authority")
	}
	after, err := root.Lstat(relative)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) ||
		after.Size() != before.Size() || after.Mode().Perm() != os.FileMode(mode) {
		return fmt.Errorf("workspace file %q changed during exact verification", relative)
	}
	return nil
}

func verifyPathFileMode(
	root *os.Root,
	relative string,
	before os.FileInfo,
	mode uint32,
) (bool, error) {
	if root == nil || before == nil {
		return false, fmt.Errorf("set workspace file mode requires exact path authority")
	}
	changed := before.Mode().Perm() != os.FileMode(mode)
	if changed {
		if err := root.Chmod(relative, os.FileMode(mode)); err != nil {
			return false, fmt.Errorf("set workspace file mode %q: %w", relative, err)
		}
	}
	if err := verifyPathFileExact(root, relative, before, mode); err != nil {
		return changed, err
	}
	return changed, nil
}

func applyDeletion(
	root *os.Root,
	relative string,
	result *ReconciliationResult,
) error {
	missingParent, err := inspectWorkspaceParents(root, relative, false)
	if err != nil {
		return err
	}
	if missingParent {
		return nil
	}
	info, err := root.Lstat(relative)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect workspace deletion %q: %w", relative, err)
	}
	if info.IsDir() {
		return fmt.Errorf("workspace deletion %q names a directory, not a file", relative)
	}
	if err := root.Remove(relative); err != nil {
		return fmt.Errorf("delete workspace file %q: %w", relative, err)
	}
	result.Changes = append(result.Changes, Change{Path: relative, Kind: ChangeDelete})
	return nil
}

func applyMoves(
	ctx context.Context,
	root *os.Root,
	moves []DesiredFile,
	result *ReconciliationResult,
) error {
	targets := make(map[string]struct{}, len(moves))
	for _, move := range moves {
		targets[move.Path] = struct{}{}
	}
	for _, move := range moves {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("apply workspace move: %w", err)
		}
		settled, err := reconcileMoveDestination(ctx, root, move, result)
		if err != nil {
			return err
		}
		if settled {
			continue
		}
		_, sourceIsTarget := targets[move.MoveFrom]
		if !sourceIsTarget {
			moved, err := applyRenameFromMatchingSource(ctx, root, move, result)
			if err != nil {
				return err
			}
			if moved {
				continue
			}
		}
		replacement := move
		replacement.MoveFrom = ""
		if err := applyFile(ctx, root, replacement, result); err != nil {
			return err
		}
	}
	for _, move := range moves {
		if _, retained := targets[move.MoveFrom]; retained {
			continue
		}
		if err := applyDeletion(root, move.MoveFrom, result); err != nil {
			return err
		}
	}
	return nil
}

func reconcileMoveDestination(
	ctx context.Context,
	root *os.Root,
	move DesiredFile,
	result *ReconciliationResult,
) (bool, error) {
	missingParent, err := inspectWorkspaceParents(root, move.Path, false)
	if err != nil || missingParent {
		return false, err
	}
	before, err := root.Lstat(move.Path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect workspace move destination %q: %w", move.Path, err)
	}
	if !before.Mode().IsRegular() {
		return false, nil
	}
	file, matches, err := openRegularFileContentMatch(ctx, root, move.Path, before, move.Content)
	if err != nil {
		return false, err
	}
	if !matches {
		closeWorkspaceHandle(result, move.Path, file)
		return false, nil
	}
	if file == nil {
		changed, err := verifyPathFileMode(root, move.Path, before, move.Mode)
		if err != nil {
			return false, err
		}
		if changed {
			result.Changes = append(result.Changes, Change{Path: move.Path, Kind: ChangeReplace})
		}
		return true, nil
	}
	changed, err := verifyOpenFileMode(root, move.Path, file, before, move.Mode, result)
	if err != nil {
		return false, err
	}
	if changed {
		result.Changes = append(result.Changes, Change{Path: move.Path, Kind: ChangeReplace})
	}
	return true, nil
}

func applyRenameFromMatchingSource(
	ctx context.Context,
	root *os.Root,
	move DesiredFile,
	result *ReconciliationResult,
) (bool, error) {
	missingParent, err := inspectWorkspaceParents(root, move.MoveFrom, false)
	if err != nil || missingParent {
		return false, err
	}
	before, err := root.Lstat(move.MoveFrom)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect workspace move source %q: %w", move.MoveFrom, err)
	}
	if !before.Mode().IsRegular() {
		return false, nil
	}
	file, matches, err := openRegularFileContentMatch(ctx, root, move.MoveFrom, before, move.Content)
	if err != nil {
		return false, err
	}
	if !matches {
		closeWorkspaceHandle(result, move.MoveFrom, file)
		return false, nil
	}
	if before.Mode().Perm() != os.FileMode(move.Mode) {
		closeWorkspaceHandle(result, move.MoveFrom, file)
		return false, nil
	}
	if _, err := inspectWorkspaceParents(root, move.Path, true); err != nil {
		closeWorkspaceHandle(result, move.MoveFrom, file)
		return false, err
	}
	destination, err := root.Lstat(move.Path)
	if err == nil {
		if destination.IsDir() {
			closeWorkspaceHandle(result, move.MoveFrom, file)
			return false, fmt.Errorf("workspace move destination %q is a directory", move.Path)
		}
		if !destination.Mode().IsRegular() && destination.Mode()&os.ModeSymlink == 0 {
			closeWorkspaceHandle(result, move.MoveFrom, file)
			return false, fmt.Errorf("workspace move destination %q is a special filesystem node", move.Path)
		}
	} else if !os.IsNotExist(err) {
		closeWorkspaceHandle(result, move.MoveFrom, file)
		return false, fmt.Errorf("inspect workspace move destination %q: %w", move.Path, err)
	}
	if err := root.Rename(move.MoveFrom, move.Path); err != nil {
		closeWorkspaceHandle(result, move.MoveFrom, file)
		return false, fmt.Errorf("move workspace file %q to %q: %w", move.MoveFrom, move.Path, err)
	}
	result.Changes = append(result.Changes, Change{
		Path: move.Path, Kind: ChangeMove, SourcePath: move.MoveFrom,
	})
	if file == nil {
		if err := verifyPathFileExact(root, move.Path, before, move.Mode); err != nil {
			return false, err
		}
	} else {
		if _, err := verifyOpenFileMode(root, move.Path, file, before, move.Mode, result); err != nil {
			return false, err
		}
	}
	return true, nil
}

func closeWorkspaceHandle(result *ReconciliationResult, relative string, file *os.File) {
	if file == nil {
		return
	}
	if err := file.Close(); err != nil && result != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"close verified workspace handle %q: %v", relative, err,
		))
	}
}
