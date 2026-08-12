package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

// VerifyCognitionTerminalCompletionTraceAuthority binds portable terminal
// evidence to the exact completion persisted by the production seal path.
func VerifyCognitionTerminalCompletionTraceAuthority(
	seal CognitionTerminalSeal,
	completion cognition.CompletionResult,
) error {
	_, completionSHA, err := cognitionJSON(completion)
	if err != nil || completion.Validate() != nil || seal.EpisodeID == "" ||
		seal.FinalRevision.Validate() != nil || seal.FinalRevision.EpisodeID != seal.EpisodeID ||
		!validCognitionTerminalStatus(seal.Outcome) ||
		!cognitionDigestPattern.MatchString(seal.CompletionSHA256) ||
		!cognitionDigestPattern.MatchString(seal.ObligationGraphSHA256) ||
		!cognitionDigestPattern.MatchString(seal.TraceSHA256) ||
		seal.LedgerVersion == 0 || seal.WorkingSetVersion == 0 || seal.CreatedAt.IsZero() ||
		validateCognitionTerminalSealAuthority(seal) != nil ||
		completion.Revision != seal.FinalRevision || completionSHA != seal.CompletionSHA256 ||
		(seal.Outcome == CognitionEpisodeCompleted) !=
			(completion.Outcome == cognition.CompletionSatisfied) {
		return fmt.Errorf("%w: terminal completion trace authority changed", ErrCognitionConflict)
	}
	return nil
}

// VerifyCognitionWorkerTerminalActorTraceAuthority reuses the production
// AttemptRef-to-queue-authority conversion and terminal authority XOR.
func VerifyCognitionWorkerTerminalActorTraceAuthority(
	seal CognitionTerminalSeal,
	actor cognition.AttemptRef,
) error {
	authority, err := providerProcessObservationAuthority(actor)
	if err != nil || validateCognitionTerminalSealAuthority(seal) != nil ||
		seal.AuthorityKind != cognitionTerminalAuthorityWorker ||
		seal.SealedBy != authority || seal.LifecycleOperationID != "" {
		return fmt.Errorf("%w: worker terminal trace actor changed", ErrCognitionConflict)
	}
	return nil
}
