package worker

import (
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (s *directCodingSession) ApplyAndVerify(
	prepared *directCodingPreparedMutation,
) (resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil || s.runtime.svc == nil ||
		s.runtime.claim == nil || prepared == nil {
		return fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	if prepared.reconciliation == nil {
		return fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	if err := s.runtime.svc.requireWorkspaceScopeForV3Job(
		s.runtime.claim.Job,
		s.root,
	); err != nil {
		return fmt.Errorf("validate host workspace before reconciliation: %w", err)
	}
	result, err := prepared.reconciliation.ApplyVerified(
		s.runtime.ctx,
		s.observeVerifiedWorkspaceChange,
	)
	prepared.result = result
	if err != nil {
		return fmt.Errorf(
			"workspace reconciliation stopped after %d applied changes: %w",
			len(result.Changes), err,
		)
	}
	if prepared.hostVerificationProgram != nil {
		if err := s.verifyAuthoritativeTypeScriptWorkspace(
			*prepared.hostVerificationProgram,
			prepared.hostVerificationAssembly,
		); err != nil {
			return fmt.Errorf("verify exact authoritative TypeScript workspace: %w", err)
		}
	}
	return nil
}

func (s *directCodingSession) observeVerifiedWorkspaceChange(
	change workspacefacts.Change,
) {
	var operation workspaceFileOperation
	switch change.Kind {
	case workspacefacts.ChangeCreate:
		operation = workspaceFileCreate
	case workspacefacts.ChangeReplace:
		operation = workspaceFileReplace
	case workspacefacts.ChangeDelete:
		operation = workspaceFileDelete
	case workspacefacts.ChangeMove:
		operation = workspaceFileMove
	default:
		s.runtime.svc.logf("unknown verified workspace change kind=%q path=%q", change.Kind, change.Path)
		return
	}
	s.mutationJournal = append(s.mutationJournal, directCodingMutationJournalEntry{
		Path: change.Path, SourcePath: change.SourcePath, Operation: operation,
	})
	s.runtime.svc.emitWorkspaceFileChange(
		s.runtime.claim.Authority,
		string(operation),
		change.SourcePath,
		change.Path,
	)
}
