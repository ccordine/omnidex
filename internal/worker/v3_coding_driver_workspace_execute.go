package worker

import (
	"errors"
	"fmt"
)

func (s *directCodingSession) ApplyAndVerify(
	prepared *directCodingPreparedMutation,
) (verification directCodingVerification, resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil || prepared == nil {
		return directCodingVerification{}, fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	if prepared.stage == nil {
		return directCodingVerification{}, fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	defer func() {
		resultErr = errors.Join(resultErr, prepared.Cleanup())
	}()
	plan := prepared.stage.Plan()
	result, err := prepared.stage.ApplyVerified(s.runtime.ctx)
	if err != nil {
		return directCodingVerification{}, err
	}
	current := result.Snapshot
	if err := plan.VerifyExpected(current); err != nil {
		return directCodingVerification{}, err
	}
	if err := current.VerifyExact(s.runtime.ctx); err != nil {
		return directCodingVerification{}, fmt.Errorf("verify stable direct-coding workspace post-state: %w", err)
	}
	s.recordPreparedWorkspaceMutation(prepared)
	authority, err := newDirectCodingExactStateAuthority(current, plan.OwnerID)
	if err != nil {
		return directCodingVerification{}, err
	}
	receiptSHA256, err := directCodingExactStateReceiptSHA256(authority, true)
	if err != nil {
		return directCodingVerification{}, err
	}
	return directCodingVerification{
		Passed:                  true,
		ExactStateAuthorityID:   authority.ID,
		ExactStateReceiptSHA256: receiptSHA256,
	}, nil
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
