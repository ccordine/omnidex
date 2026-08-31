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
	observer VerifiedChangeObserver,
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
	boundaryRoot, err := prepared.hostAccess.captureWorkspaceRoot(prepared.root)
	if err != nil {
		return ReconciliationResult{}, fmt.Errorf("revalidate host-authoritative workspace root: %w", err)
	}
	if !os.SameFile(prepared.rootInfo, boundaryRoot) {
		return ReconciliationResult{}, fmt.Errorf(
			"host-authoritative workspace root changed after reconciliation preparation",
		)
	}
	rootFS, err := os.OpenRoot(prepared.root)
	if err != nil {
		return ReconciliationResult{}, fmt.Errorf("open authoritative workspace root: %w", err)
	}
	directory, err := rootFS.Open(".")
	if err != nil {
		return ReconciliationResult{}, errors.Join(
			fmt.Errorf("open authoritative workspace root handle: %w", err), rootFS.Close(),
		)
	}
	current, err := directory.Stat()
	if err != nil || !current.IsDir() || !os.SameFile(prepared.rootInfo, current) {
		return ReconciliationResult{}, errors.Join(
			fmt.Errorf("authoritative workspace root changed before mutation"),
			directory.Close(), rootFS.Close(),
		)
	}
	mountID, err := workspaceMountIDForHandle(directory)
	if err != nil {
		return ReconciliationResult{}, errors.Join(
			fmt.Errorf("resolve authoritative workspace root mount: %w", err),
			directory.Close(), rootFS.Close(),
		)
	}
	root := &authoritativeWorkspaceRoot{
		Root: rootFS, authorityFD: int(directory.Fd()), mountID: mountID,
	}
	result, applyErr := applyReconciliation(ctx, root, prepared.desired, observer)
	closeErr := errors.Join(directory.Close(), rootFS.Close())
	if applyErr != nil {
		return result, errors.Join(applyErr, closeErr)
	}
	if closeErr != nil {
		return result, fmt.Errorf("close authoritative workspace root: %w", closeErr)
	}
	prepared.applied = true
	return result, nil
}

func applyReconciliation(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	desired []DesiredFile,
	observer VerifiedChangeObserver,
) (ReconciliationResult, error) {
	result := ReconciliationResult{Changes: []Change{}}
	moves := make([]DesiredFile, 0)
	for _, state := range desired {
		if state.MoveFrom != "" {
			moves = append(moves, state)
		}
	}
	if err := applyMoves(ctx, root, moves, &result, observer); err != nil {
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
				if err := applyRequiredFile(ctx, root, state, &result, observer); err != nil {
					return result, err
				}
				continue
			}
			if err := applyFile(ctx, root, state, &result, observer); err != nil {
				return result, err
			}
			continue
		}
		if err := applyDeletion(ctx, root, state.Path, &result, observer); err != nil {
			return result, err
		}
	}
	return result, nil
}

func applyRequiredFile(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	state DesiredFile,
	result *ReconciliationResult,
	observer VerifiedChangeObserver,
) error {
	missingParent, err := inspectWorkspaceParents(ctx, root, state.Path, false, nil, nil)
	if err != nil {
		return err
	}
	if !missingParent {
		info, err := root.Lstat(state.Path)
		if err == nil {
			if info.Mode().IsRegular() {
				return nil
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect required workspace file %q: %w", state.Path, err)
		}
	}
	state.PreserveExisting = false
	return applyFile(ctx, root, state, result, observer)
}

func applyFile(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	state DesiredFile,
	result *ReconciliationResult,
	observer VerifiedChangeObserver,
) (resultErr error) {
	if _, err := inspectWorkspaceParents(ctx, root, state.Path, true, result, observer); err != nil {
		return err
	}
	before, err := root.Lstat(state.Path)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect workspace file %q: %w", state.Path, err)
	}
	if existed && before.Mode().IsRegular() {
		file, matches, err := openRegularFileContentMatch(ctx, root, state.Path, before, state.Content)
		if err != nil {
			return err
		}
		if matches {
			if before.Mode().Perm() == os.FileMode(state.Mode) {
				if file == nil {
					return verifyPathFileExact(root, state.Path, before, state.Mode)
				}
				return verifyOpenFileExact(ctx, root, state.Path, file, before, state.Mode)
			}
		}
		if err := closeWorkspaceHandle(state.Path, file); err != nil {
			return err
		}
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
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("install workspace file %q: %w", state.Path, err)
	}
	if existed && before.IsDir() {
		if err := removeWorkspaceEntry(ctx, root, state.Path); err != nil {
			return fmt.Errorf("replace workspace directory %q with a file: %w", state.Path, err)
		}
	}
	if err := root.Rename(temporary, state.Path); err != nil {
		return fmt.Errorf("install workspace file %q: %w", state.Path, err)
	}
	keepTemporary = false
	kind := ChangeCreate
	if existed {
		kind = ChangeReplace
	}
	after, err := root.Lstat(state.Path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(temporaryInfo, after) ||
		after.Size() != int64(len(state.Content)) || after.Mode().Perm() != os.FileMode(state.Mode) {
		return fmt.Errorf("installed workspace file %q differs from desired bytes or permissions", state.Path)
	}
	recordVerifiedChange(result, observer, Change{Path: state.Path, Kind: kind})
	return nil
}

func verifyOpenFileExact(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	relative string,
	file *os.File,
	before os.FileInfo,
	mode uint32,
) error {
	if ctx == nil || root == nil || file == nil || before == nil {
		return fmt.Errorf("verify workspace file requires exact open-file authority")
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(
			fmt.Errorf("verify workspace file %q: %w", relative, err),
			closeWorkspaceHandle(relative, file),
		)
	}
	fileInfo, fileErr := file.Stat()
	pathInfo, pathErr := root.Lstat(relative)
	closeErr := closeWorkspaceHandle(relative, file)
	if fileErr != nil || pathErr != nil || !fileInfo.Mode().IsRegular() ||
		!os.SameFile(before, fileInfo) || !os.SameFile(before, pathInfo) ||
		before.Mode().Perm() != os.FileMode(mode) ||
		fileInfo.Mode().Perm() != os.FileMode(mode) ||
		pathInfo.Mode().Perm() != os.FileMode(mode) {
		return errors.Join(
			fmt.Errorf("workspace file %q differs from desired state", relative),
			closeErr,
		)
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func verifyPathFileExact(
	root *authoritativeWorkspaceRoot,
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

func applyDeletion(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	relative string,
	result *ReconciliationResult,
	observer VerifiedChangeObserver,
) error {
	missingParent, err := inspectWorkspaceParents(ctx, root, relative, false, nil, nil)
	if err != nil {
		return err
	}
	if missingParent {
		return nil
	}
	_, err = root.Lstat(relative)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect workspace deletion %q: %w", relative, err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete workspace file %q: %w", relative, err)
	}
	if err := removeWorkspaceEntry(ctx, root, relative); err != nil {
		return fmt.Errorf("delete workspace file %q: %w", relative, err)
	}
	if err := verifyAndRecordWorkspaceDeletion(root, result, observer, relative); err != nil {
		return fmt.Errorf("verify deleted workspace path %q: %w", relative, err)
	}
	return nil
}

func applyMoves(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	moves []DesiredFile,
	result *ReconciliationResult,
	observer VerifiedChangeObserver,
) error {
	targets := make(map[string]struct{}, len(moves))
	for _, move := range moves {
		targets[move.Path] = struct{}{}
	}
	for _, move := range moves {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("apply workspace move: %w", err)
		}
		settled, err := reconcileMoveDestination(ctx, root, move)
		if err != nil {
			return err
		}
		if settled {
			continue
		}
		_, sourceIsTarget := targets[move.MoveFrom]
		if !sourceIsTarget {
			moved, err := applyRenameFromMatchingSource(ctx, root, move, result, observer)
			if err != nil {
				return err
			}
			if moved {
				continue
			}
		}
		replacement := move
		replacement.MoveFrom = ""
		if err := applyFile(ctx, root, replacement, result, observer); err != nil {
			return err
		}
	}
	for _, move := range moves {
		if _, retained := targets[move.MoveFrom]; retained {
			continue
		}
		if err := applyDeletion(ctx, root, move.MoveFrom, result, observer); err != nil {
			return err
		}
	}
	return nil
}

func reconcileMoveDestination(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	move DesiredFile,
) (bool, error) {
	missingParent, err := inspectWorkspaceParents(ctx, root, move.Path, false, nil, nil)
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
		if err := closeWorkspaceHandle(move.Path, file); err != nil {
			return false, err
		}
		return false, nil
	}
	if before.Mode().Perm() != os.FileMode(move.Mode) {
		if err := closeWorkspaceHandle(move.Path, file); err != nil {
			return false, err
		}
		return false, nil
	}
	if file == nil {
		return true, verifyPathFileExact(root, move.Path, before, move.Mode)
	}
	return true, verifyOpenFileExact(ctx, root, move.Path, file, before, move.Mode)
}

func applyRenameFromMatchingSource(
	ctx context.Context,
	root *authoritativeWorkspaceRoot,
	move DesiredFile,
	result *ReconciliationResult,
	observer VerifiedChangeObserver,
) (bool, error) {
	missingParent, err := inspectWorkspaceParents(ctx, root, move.MoveFrom, false, nil, nil)
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
		if err := closeWorkspaceHandle(move.MoveFrom, file); err != nil {
			return false, err
		}
		return false, nil
	}
	if before.Mode().Perm() != os.FileMode(move.Mode) {
		if err := closeWorkspaceHandle(move.MoveFrom, file); err != nil {
			return false, err
		}
		return false, nil
	}
	if _, err := inspectWorkspaceParents(ctx, root, move.Path, true, result, observer); err != nil {
		return false, errors.Join(err, closeWorkspaceHandle(move.MoveFrom, file))
	}
	destination, err := root.Lstat(move.Path)
	if err == nil {
		if destination.IsDir() {
			if err := removeWorkspaceEntry(ctx, root, move.Path); err != nil {
				return false, errors.Join(
					fmt.Errorf("replace workspace move destination %q: %w", move.Path, err),
					closeWorkspaceHandle(move.MoveFrom, file),
				)
			}
		}
	} else if !os.IsNotExist(err) {
		return false, errors.Join(
			fmt.Errorf("inspect workspace move destination %q: %w", move.Path, err),
			closeWorkspaceHandle(move.MoveFrom, file),
		)
	}
	if err := ctx.Err(); err != nil {
		return false, errors.Join(
			fmt.Errorf("move workspace file %q to %q: %w", move.MoveFrom, move.Path, err),
			closeWorkspaceHandle(move.MoveFrom, file),
		)
	}
	if err := root.Rename(move.MoveFrom, move.Path); err != nil {
		return false, errors.Join(
			fmt.Errorf("move workspace file %q to %q: %w", move.MoveFrom, move.Path, err),
			closeWorkspaceHandle(move.MoveFrom, file),
		)
	}
	if file == nil {
		if err := verifyPathFileExact(root, move.Path, before, move.Mode); err != nil {
			return false, err
		}
	} else {
		if err := verifyOpenFileExact(ctx, root, move.Path, file, before, move.Mode); err != nil {
			return false, err
		}
	}
	if err := verifyWorkspacePathAbsent(root, move.MoveFrom); err != nil {
		return false, fmt.Errorf(
			"verify workspace move source %q is absent: %w", move.MoveFrom, err,
		)
	}
	recordVerifiedChange(result, observer, Change{
		Path: move.Path, Kind: ChangeMove, SourcePath: move.MoveFrom,
	})
	return true, nil
}

func closeWorkspaceHandle(relative string, file *os.File) error {
	if file == nil {
		return nil
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close verified workspace handle %q: %w", relative, err)
	}
	return nil
}
