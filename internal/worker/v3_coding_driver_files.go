package worker

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"errors"
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func directCodingDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

type directCodingPreparedMutation struct {
	assembly      directCodingAssembly
	stage         *workspacefacts.StagedReconciliation
	recorded      bool
}

func (prepared *directCodingPreparedMutation) Cleanup() error {
	if prepared == nil || prepared.stage == nil {
		return nil
	}
	err := prepared.stage.Cleanup()
	if err == nil {
		prepared.stage = nil
	}
	return err
}

func (s *directCodingSession) PrepareAssembly(
	assembly directCodingAssembly,
) (_ *directCodingPreparedMutation, resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil {
		return nil, fmt.Errorf("workspace mutation preparation requires one active session")
	}
	if err := assembly.validate(); err != nil {
		return nil, err
	}
	desired, err := s.directCodingAssemblyDesiredStates(assembly)
	if err != nil {
		return nil, err
	}
	ownerID, err := directCodingAssemblyOwnerID(assembly)
	if err != nil {
		return nil, err
	}
	prepared := &directCodingPreparedMutation{assembly: assembly}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, prepared.Cleanup())
		}
	}()
	plan, err := workspacefacts.PlanReconciliation(s.runtime.ctx, s.root, ownerID, desired)
	if err != nil {
		return nil, fmt.Errorf("plan direct-coding workspace reconciliation: %w", err)
	}
	stage, err := workspacefacts.StageReconciliation(s.runtime.ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("stage direct-coding workspace reconciliation: %w", err)
	}
	prepared.stage = stage
	return prepared, nil
}

func (s *directCodingSession) directCodingAssemblyDesiredStates(
	assembly directCodingAssembly,
) ([]workspacefacts.DesiredFile, error) {
	if len(assembly.Files)+len(assembly.DeletePaths) > workspacefacts.MaxReconciliationFiles {
		return nil, fmt.Errorf(
			"direct-coding workspace delta exceeds the %d-file transaction limit",
			workspacefacts.MaxReconciliationFiles,
		)
	}
	desired := make([]workspacefacts.DesiredFile, 0, len(assembly.Files)+len(assembly.DeletePaths))
	for _, task := range assembly.Files {
		if err := rejectDirectCodingProtectedMutation(task.Path, s.protectedPaths); err != nil {
			return nil, err
		}
		state := workspacefacts.DesiredFile{
			Path: task.Path, Present: true,
			Content: append([]byte(nil), task.Content...), Mode: task.Mode,
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

func directCodingAssemblyOwnerID(assembly directCodingAssembly) (string, error) {
	raw, err := json.Marshal(assembly)
	if err != nil {
		return "", fmt.Errorf("encode direct-coding assembly identity: %w", err)
	}
	return "coding_" + directCodingDigest(string(raw)), nil
}
