package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func directCodingDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

type directCodingPreparedMutation struct {
	reconciliation *workspacefacts.PreparedReconciliation
	result         workspacefacts.ReconciliationResult
	recorded       bool
}

func (s *directCodingSession) PrepareAssembly(
	assembly directCodingAssembly,
) (*directCodingPreparedMutation, error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil {
		return nil, fmt.Errorf("workspace mutation preparation requires one active session")
	}
	desired, err := s.directCodingAssemblyDesiredStates(assembly)
	if err != nil {
		return nil, err
	}
	s.plannedFiles = 0
	s.plannedDeletes = 0
	for _, state := range desired {
		if state.Present {
			s.plannedFiles++
		} else {
			s.plannedDeletes++
		}
	}
	reconciliation, err := workspacefacts.PrepareReconciliation(s.runtime.ctx, s.root, desired)
	if err != nil {
		return nil, fmt.Errorf("prepare direct-coding workspace reconciliation: %w", err)
	}
	return &directCodingPreparedMutation{reconciliation: reconciliation}, nil
}

func (s *directCodingSession) directCodingAssemblyDesiredStates(
	assembly directCodingAssembly,
) ([]workspacefacts.DesiredFile, error) {
	desired := make([]workspacefacts.DesiredFile, 0, len(assembly.Files)+len(assembly.RequiredPaths)+len(assembly.DeletePaths))
	exactFiles := make(map[string]struct{}, len(assembly.Files))
	desiredPaths := make(map[string]struct{}, len(assembly.Files)+len(assembly.RequiredPaths)+len(assembly.DeletePaths))
	deletions := make(map[string]struct{}, len(assembly.DeletePaths))
	for _, path := range assembly.DeletePaths {
		deletions[path] = struct{}{}
	}
	for _, task := range assembly.Files {
		if directCodingPathProtected(task.Path, s.protectedPaths) {
			return nil, fmt.Errorf(
				"compiled file %q conflicts with accepted preservation authority", task.Path,
			)
		}
		for deletion := range deletions {
			if task.Path == deletion || directCodingTargetTreeFileAncestor(deletion, task.Path) {
				return nil, fmt.Errorf(
					"compiled file %q conflicts with accepted deletion %q", task.Path, deletion,
				)
			}
		}
		state := workspacefacts.DesiredFile{
			Path: task.Path, Present: true,
			Content: append([]byte(nil), task.Content...), Mode: task.Mode,
			MoveFrom: task.MoveFrom,
		}
		if state.MoveFrom != "" && directCodingPathProtected(state.MoveFrom, s.protectedPaths) {
			state.MoveFrom = ""
		}
		desired = append(desired, state)
		exactFiles[task.Path] = struct{}{}
		desiredPaths[task.Path] = struct{}{}
	}
	for _, required := range assembly.RequiredPaths {
		if _, exists := exactFiles[required]; exists {
			continue
		}
		if directCodingPathProtected(required, s.protectedPaths) {
			return nil, fmt.Errorf(
				"required file %q conflicts with accepted preservation authority", required,
			)
		}
		for deletion := range deletions {
			if directCodingTargetTreeFileHierarchyConflict(required, deletion) {
				return nil, fmt.Errorf(
					"required file %q conflicts with accepted deletion %q", required, deletion,
				)
			}
		}
		if directCodingPathConflictsWithSet(required, desiredPaths) {
			return nil, fmt.Errorf(
				"required file %q crosses an accepted file hierarchy", required,
			)
		}
		desired = append(desired, workspacefacts.DesiredFile{
			Path: required, Present: true, Content: []byte{}, Mode: 0o644,
			PreserveExisting: true,
		})
		desiredPaths[required] = struct{}{}
	}
	for _, path := range assembly.DeletePaths {
		if directCodingPathProtected(path, s.protectedPaths) {
			return nil, fmt.Errorf(
				"deleted file %q conflicts with accepted preservation authority", path,
			)
		}
		redundant := false
		for desiredPath := range desiredPaths {
			if !directCodingTargetTreeFileHierarchyConflict(path, desiredPath) {
				continue
			}
			if directCodingTargetTreeFileAncestor(desiredPath, path) {
				redundant = true
				break
			}
			return nil, fmt.Errorf(
				"deleted file %q conflicts with accepted file %q", path, desiredPath,
			)
		}
		if redundant {
			continue
		}
		desired = append(desired, workspacefacts.DesiredFile{Path: path})
		desiredPaths[path] = struct{}{}
	}
	return desired, nil
}

func directCodingPathConflictsWithSet(candidate string, paths map[string]struct{}) bool {
	for existing := range paths {
		if directCodingTargetTreeFileHierarchyConflict(candidate, existing) {
			return true
		}
	}
	return false
}
