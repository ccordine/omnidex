package worker

import (
	"fmt"
)

func (s *directCodingSession) ApplyAndVerify(
	prepared *directCodingPreparedMutation,
) (resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil || prepared == nil {
		return fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	if prepared.stage == nil {
		return fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	defer func() {
		if err := prepared.Cleanup(); err != nil && s.runtime.svc.logger != nil {
			s.runtime.svc.logger.Printf("workspace mutation staging cleanup failed after terminal apply state: %v", err)
		}
	}()
	plan := prepared.stage.Plan()
	result, err := prepared.stage.ApplyVerified(s.runtime.ctx)
	if err != nil {
		return err
	}
	current := result.Snapshot
	if err := plan.VerifyExpected(current); err != nil {
		return err
	}
	if err := current.VerifyExact(s.runtime.ctx); err != nil {
		return fmt.Errorf("verify stable direct-coding workspace post-state: %w", err)
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
	for _, transition := range prepared.stage.Plan().Files {
		operation := workspaceFileReplace
		switch {
		case !transition.Source.Present:
			operation = workspaceFileCreate
		case !transition.Expected.Present:
			operation = workspaceFileDelete
		}
		s.mutationJournal = append(s.mutationJournal, directCodingMutationJournalEntry{
			Path: transition.Path, Operation: operation,
		})
	}
	prepared.recorded = true
}
