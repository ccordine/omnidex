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
	desired := make([]workspacefacts.DesiredFile, 0, len(assembly.Files)+len(assembly.DeletePaths))
	for _, task := range assembly.Files {
		if err := rejectDirectCodingProtectedMutation(task.Path, s.protectedPaths); err != nil {
			return nil, err
		}
		if task.MoveFrom != "" {
			if err := rejectDirectCodingProtectedMutation(task.MoveFrom, s.protectedPaths); err != nil {
				return nil, err
			}
		}
		state := workspacefacts.DesiredFile{
			Path: task.Path, Present: true,
			Content: append([]byte(nil), task.Content...), Mode: task.Mode,
			MoveFrom: task.MoveFrom,
		}
		desired = append(desired, state)
	}
	for _, path := range assembly.DeletePaths {
		if err := rejectDirectCodingProtectedMutation(path, s.protectedPaths); err != nil {
			return nil, err
		}
		desired = append(desired, workspacefacts.DesiredFile{Path: path})
	}
	return desired, nil
}
