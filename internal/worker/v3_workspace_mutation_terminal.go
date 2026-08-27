package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *directCodingSession) bindWorkspaceMutationTerminal(
	command queue.WorkspaceMutationCommand,
	result queue.WorkspaceMutationResult,
	verification *directCodingVerification,
) error {
	if s == nil || s.runtime == nil || s.runtime.svc == nil ||
		s.runtime.svc.repo == nil || s.runtime.claim == nil || verification == nil {
		return fmt.Errorf("bind workspace mutation terminal requires one active coding session")
	}
	authority := s.runtime.claim.Authority
	snapshot, err := s.runtime.svc.repo.CurrentWorkspaceMutation(
		s.runtime.ctx, authority.JobID, authority.Generation,
	)
	if err != nil {
		return fmt.Errorf("load terminal workspace mutation receipt: %w", err)
	}
	if snapshot == nil || snapshot.Terminal == nil ||
		snapshot.OperationID != result.OperationID ||
		snapshot.Command.Plan.ID != command.Plan.ID ||
		snapshot.Command.StepID != command.StepID ||
		!equalWorkspaceMutationResults(snapshot.Terminal.Result, result) {
		return fmt.Errorf("terminal workspace mutation receipt differs from executed command")
	}
	verification.MutationOperationID = snapshot.OperationID
	verification.MutationReceiptSHA256 = snapshot.Terminal.ReceiptSHA256
	return nil
}

func equalWorkspaceMutationResults(
	left, right queue.WorkspaceMutationResult,
) bool {
	if left.OperationID != right.OperationID || left.Status != right.Status ||
		left.MutationEvidenceID != right.MutationEvidenceID ||
		left.VerificationEvidenceID != right.VerificationEvidenceID ||
		left.VerificationSucceeded != right.VerificationSucceeded ||
		len(left.CommandEvidenceIDs) != len(right.CommandEvidenceIDs) {
		return false
	}
	for index := range left.CommandEvidenceIDs {
		if left.CommandEvidenceIDs[index] != right.CommandEvidenceIDs[index] {
			return false
		}
	}
	return true
}
