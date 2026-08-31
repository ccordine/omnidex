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
			continue
		}
		if _, forbidden := deletions[task.Path]; forbidden {
			continue
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
			continue
		}
		if _, forbidden := deletions[required]; forbidden {
			continue
		}
		if directCodingPathConflictsWithSet(required, desiredPaths) {
			continue
		}
		desired = append(desired, workspacefacts.DesiredFile{
			Path: required, Present: true, Content: []byte{}, Mode: 0o644,
			PreserveExisting: true,
		})
		desiredPaths[required] = struct{}{}
	}
	for _, path := range assembly.DeletePaths {
		if directCodingPathProtected(path, s.protectedPaths) {
			continue
		}
		if directCodingPathConflictsWithSet(path, desiredPaths) {
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
