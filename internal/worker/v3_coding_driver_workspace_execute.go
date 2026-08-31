package worker

import (
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (s *directCodingSession) ApplyAndVerify(
	prepared *directCodingPreparedMutation,
) (resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil || prepared == nil {
		return fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	if prepared.reconciliation == nil {
		return fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	result, err := prepared.reconciliation.ApplyVerified(s.runtime.ctx)
	if err != nil {
		return err
	}
	prepared.result = result
	for _, warning := range result.Warnings {
		s.runtime.svc.logf("workspace mutation cleanup warning: %s", warning)
	}
	s.recordPreparedWorkspaceMutation(prepared)
	return nil
}

func (s *directCodingSession) recordPreparedWorkspaceMutation(
	prepared *directCodingPreparedMutation,
) {
	if prepared.recorded {
		return
	}
	for _, change := range prepared.result.Changes {
		operation := workspaceFileReplace
		switch change.Kind {
		case workspacefacts.ChangeCreate:
			operation = workspaceFileCreate
		case workspacefacts.ChangeDelete:
			operation = workspaceFileDelete
		case workspacefacts.ChangeMove:
			operation = workspaceFileMove
		}
		s.mutationJournal = append(s.mutationJournal, directCodingMutationJournalEntry{
			Path: change.Path, Operation: operation,
		})
	}
	prepared.recorded = true
}
