package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type fileMutationJournal struct {
	target    string
	backup    string
	installed bool
}

type moveMutationJournal struct {
	source            string
	destination       string
	stagedSource      string
	destinationBackup string
	sourceInfo        os.FileInfo
	sourceMode        os.FileMode
	placed            bool
}

type modeMutationJournal struct {
	target       string
	previousMode os.FileMode
}

type reconciliationJournal struct {
	files       []fileMutationJournal
	moves       []moveMutationJournal
	modes       []modeMutationJournal
	createdDirs []string
}

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
	currentRoot, err := exactRootDirectory(prepared.root)
	if err != nil {
		return ReconciliationResult{}, err
	}
	if !os.SameFile(prepared.rootInfo, currentRoot) {
		return ReconciliationResult{}, fmt.Errorf("authoritative workspace root changed before mutation")
	}
	result, journal, err := applyReconciliation(ctx, prepared.root, prepared.desired)
	if err != nil {
		rollbackErr := rollbackReconciliation(prepared.root, journal)
		return ReconciliationResult{}, errors.Join(err, rollbackErr)
	}
	if err := discardReconciliationBackups(journal); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	prepared.applied = true
	return result, nil
}

func applyReconciliation(
	ctx context.Context,
	root string,
	desired []DesiredFile,
) (ReconciliationResult, reconciliationJournal, error) {
	result := ReconciliationResult{Changes: []Change{}}
	journal := reconciliationJournal{}
	moves := make([]DesiredFile, 0)
	for _, state := range desired {
		if state.MoveFrom != "" {
			moves = append(moves, state)
		}
	}
	if err := applyMoves(ctx, root, moves, &result, &journal); err != nil {
		return ReconciliationResult{}, journal, err
	}
	for _, state := range desired {
		if state.MoveFrom != "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return ReconciliationResult{}, journal, fmt.Errorf("apply workspace reconciliation: %w", err)
		}
		if state.Present {
			if err := applyFile(ctx, root, state, &result, &journal); err != nil {
				return ReconciliationResult{}, journal, err
			}
			continue
		}
		if err := applyDeletion(root, state.Path, &result, &journal); err != nil {
			return ReconciliationResult{}, journal, err
		}
	}
	return result, journal, nil
}

func applyFile(
	ctx context.Context,
	root string,
	state DesiredFile,
	result *ReconciliationResult,
	journal *reconciliationJournal,
) error {
	created, _, err := inspectWorkspaceParents(root, state.Path, true)
	journal.createdDirs = append(journal.createdDirs, created...)
	if err != nil {
		return err
	}
	target := workspacePath(root, state.Path)
	before, err := os.Lstat(target)
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
	if existed && regularFileMatches(ctx, target, before, state.Content, state.Mode) {
		return nil
	}
	temporary, temporaryInfo, err := writePreparedFile(ctx, filepath.Dir(target), state.Content, state.Mode)
	if err != nil {
		return fmt.Errorf("prepare workspace file %q: %w", state.Path, err)
	}
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = removeIfPresent(temporary)
		}
	}()
	entry := fileMutationJournal{target: target}
	if existed {
		entry.backup, err = reserveSiblingPath(filepath.Dir(target), ".omnidex-backup-")
		if err != nil {
			return fmt.Errorf("reserve workspace backup for %q: %w", state.Path, err)
		}
		if err := os.Rename(target, entry.backup); err != nil {
			return fmt.Errorf("backup workspace file %q: %w", state.Path, err)
		}
	}
	journal.files = append(journal.files, entry)
	journalIndex := len(journal.files) - 1
	if err := os.Rename(temporary, target); err != nil {
		return fmt.Errorf("install workspace file %q: %w", state.Path, err)
	}
	keepTemporary = false
	journal.files[journalIndex].installed = true
	after, err := os.Lstat(target)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(temporaryInfo, after) || after.Size() != int64(len(state.Content)) || after.Mode().Perm() != os.FileMode(state.Mode) {
		return fmt.Errorf("installed workspace file %q differs from desired bytes or permissions", state.Path)
	}
	kind := ChangeCreate
	if existed {
		kind = ChangeReplace
	}
	result.Changes = append(result.Changes, Change{Path: state.Path, Kind: kind})
	return nil
}

func applyDeletion(
	root string,
	relative string,
	result *ReconciliationResult,
	journal *reconciliationJournal,
) error {
	_, missingParent, err := inspectWorkspaceParents(root, relative, false)
	if err != nil {
		return err
	}
	if missingParent {
		return nil
	}
	target := workspacePath(root, relative)
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect workspace deletion %q: %w", relative, err)
	}
	if info.IsDir() {
		return fmt.Errorf("workspace deletion %q names a directory, not a file", relative)
	}
	backup, err := reserveSiblingPath(filepath.Dir(target), ".omnidex-backup-")
	if err != nil {
		return fmt.Errorf("reserve workspace deletion backup for %q: %w", relative, err)
	}
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("delete workspace file %q: %w", relative, err)
	}
	journal.files = append(journal.files, fileMutationJournal{target: target, backup: backup})
	result.Changes = append(result.Changes, Change{Path: relative, Kind: ChangeDelete})
	return nil
}

func applyMoves(
	ctx context.Context,
	root string,
	moves []DesiredFile,
	result *ReconciliationResult,
	journal *reconciliationJournal,
) error {
	if len(moves) == 0 {
		return nil
	}
	type sourceState struct {
		move    DesiredFile
		info    os.FileInfo
		present bool
	}
	sources := make([]sourceState, len(moves))
	presentPaths := make(map[string]struct{}, len(moves))
	for index, move := range moves {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, missingParent, err := inspectWorkspaceParents(root, move.MoveFrom, false)
		if err != nil {
			return err
		}
		if missingParent {
			sources[index] = sourceState{move: move}
			continue
		}
		source := workspacePath(root, move.MoveFrom)
		info, err := os.Lstat(source)
		if os.IsNotExist(err) {
			sources[index] = sourceState{move: move}
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect workspace move source %q: %w", move.MoveFrom, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("workspace move source %q is not a regular file", move.MoveFrom)
		}
		sources[index] = sourceState{move: move, info: info, present: true}
		presentPaths[move.MoveFrom] = struct{}{}
	}
	pending := make([]sourceState, 0, len(sources))
	settled := make([]sourceState, 0, len(sources))
	for _, source := range sources {
		if source.present {
			pending = append(pending, source)
			continue
		}
		_, missingParent, err := inspectWorkspaceParents(root, source.move.Path, false)
		if err != nil {
			return err
		}
		if missingParent {
			return fmt.Errorf(
				"workspace move %q to %q has neither source nor destination",
				source.move.MoveFrom, source.move.Path,
			)
		}
		destination, err := os.Lstat(workspacePath(root, source.move.Path))
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"workspace move %q to %q has neither source nor destination",
				source.move.MoveFrom, source.move.Path,
			)
		}
		if err != nil {
			return fmt.Errorf("inspect workspace move destination %q: %w", source.move.Path, err)
		}
		if !destination.Mode().IsRegular() {
			return fmt.Errorf("workspace move destination %q is not a regular file", source.move.Path)
		}
		if _, willMove := presentPaths[source.move.Path]; willMove {
			return fmt.Errorf(
				"workspace move destination %q is also a pending move source",
				source.move.Path,
			)
		}
		source.info = destination
		settled = append(settled, source)
	}
	journalStart := len(journal.moves)
	for _, source := range pending {
		absolute := workspacePath(root, source.move.MoveFrom)
		staged, err := reserveSiblingPath(filepath.Dir(absolute), ".omnidex-move-")
		if err != nil {
			return fmt.Errorf("reserve workspace move source %q: %w", source.move.MoveFrom, err)
		}
		if err := os.Rename(absolute, staged); err != nil {
			return fmt.Errorf("stage workspace move source %q: %w", source.move.MoveFrom, err)
		}
		journal.moves = append(journal.moves, moveMutationJournal{
			source: absolute, destination: workspacePath(root, source.move.Path),
			stagedSource: staged, sourceInfo: source.info, sourceMode: source.info.Mode().Perm(),
		})
	}
	for index, source := range pending {
		move := source.move
		created, _, err := inspectWorkspaceParents(root, move.Path, true)
		journal.createdDirs = append(journal.createdDirs, created...)
		if err != nil {
			return err
		}
		entry := &journal.moves[journalStart+index]
		destinationInfo, err := os.Lstat(entry.destination)
		if err == nil {
			if destinationInfo.IsDir() {
				return fmt.Errorf("workspace move destination %q is a directory", move.Path)
			}
			entry.destinationBackup, err = reserveSiblingPath(filepath.Dir(entry.destination), ".omnidex-backup-")
			if err != nil {
				return fmt.Errorf("reserve workspace move destination %q: %w", move.Path, err)
			}
			if err := os.Rename(entry.destination, entry.destinationBackup); err != nil {
				return fmt.Errorf("backup workspace move destination %q: %w", move.Path, err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect workspace move destination %q: %w", move.Path, err)
		}
		if err := os.Rename(entry.stagedSource, entry.destination); err != nil {
			return fmt.Errorf("move workspace file %q to %q: %w", move.MoveFrom, move.Path, err)
		}
		entry.placed = true
		if err := os.Chmod(entry.destination, os.FileMode(move.Mode)); err != nil {
			return fmt.Errorf("set workspace move destination mode %q: %w", move.Path, err)
		}
		after, err := os.Lstat(entry.destination)
		if err != nil || !after.Mode().IsRegular() || !os.SameFile(entry.sourceInfo, after) || after.Mode().Perm() != os.FileMode(move.Mode) {
			return fmt.Errorf("workspace move destination %q differs from desired state", move.Path)
		}
		result.Changes = append(result.Changes, Change{
			Path: move.Path, Kind: ChangeMove, SourcePath: move.MoveFrom,
		})
	}
	for _, source := range settled {
		move := source.move
		if source.info.Mode().Perm() == os.FileMode(move.Mode) {
			continue
		}
		target := workspacePath(root, move.Path)
		journal.modes = append(journal.modes, modeMutationJournal{
			target: target, previousMode: source.info.Mode().Perm(),
		})
		if err := os.Chmod(target, os.FileMode(move.Mode)); err != nil {
			return fmt.Errorf("set settled workspace move destination mode %q: %w", move.Path, err)
		}
		after, err := os.Lstat(target)
		if err != nil || !after.Mode().IsRegular() || !os.SameFile(source.info, after) || after.Mode().Perm() != os.FileMode(move.Mode) {
			return fmt.Errorf("workspace move destination %q differs from desired permissions", move.Path)
		}
		result.Changes = append(result.Changes, Change{Path: move.Path, Kind: ChangeReplace})
	}
	return nil
}

func rollbackReconciliation(root string, journal reconciliationJournal) error {
	var failures []error
	for index := len(journal.files) - 1; index >= 0; index-- {
		entry := journal.files[index]
		if entry.installed {
			if err := removeIfPresent(entry.target); err != nil {
				failures = append(failures, fmt.Errorf("remove installed workspace file: %w", err))
				continue
			}
		}
		if entry.backup != "" {
			if err := os.Rename(entry.backup, entry.target); err != nil {
				failures = append(failures, fmt.Errorf("restore workspace backup: %w", err))
			}
		}
	}
	for index := len(journal.moves) - 1; index >= 0; index-- {
		entry := &journal.moves[index]
		if entry.placed {
			if err := os.Rename(entry.destination, entry.stagedSource); err != nil {
				failures = append(failures, fmt.Errorf("unstage workspace move destination: %w", err))
				continue
			}
			entry.placed = false
			if err := os.Chmod(entry.stagedSource, entry.sourceMode); err != nil {
				failures = append(failures, fmt.Errorf("restore workspace move source mode: %w", err))
			}
		}
		if entry.destinationBackup != "" {
			if err := os.Rename(entry.destinationBackup, entry.destination); err != nil {
				failures = append(failures, fmt.Errorf("restore workspace move destination: %w", err))
			}
		}
	}
	for index := len(journal.moves) - 1; index >= 0; index-- {
		entry := journal.moves[index]
		if entry.placed {
			continue
		}
		if _, err := os.Lstat(entry.stagedSource); err == nil {
			if err := os.Rename(entry.stagedSource, entry.source); err != nil {
				failures = append(failures, fmt.Errorf("restore workspace move source: %w", err))
			}
		} else if !os.IsNotExist(err) {
			failures = append(failures, fmt.Errorf("inspect staged workspace move source: %w", err))
		}
	}
	for index := len(journal.modes) - 1; index >= 0; index-- {
		entry := journal.modes[index]
		if err := os.Chmod(entry.target, entry.previousMode); err != nil {
			failures = append(failures, fmt.Errorf("restore workspace file mode: %w", err))
		}
	}
	if err := removeCreatedDirectories(root, journal.createdDirs); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func discardReconciliationBackups(journal reconciliationJournal) error {
	var failures []error
	for _, entry := range journal.files {
		if entry.backup != "" {
			if err := removeIfPresent(entry.backup); err != nil {
				failures = append(failures, err)
			}
		}
	}
	for _, entry := range journal.moves {
		if entry.destinationBackup != "" {
			if err := removeIfPresent(entry.destinationBackup); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}
