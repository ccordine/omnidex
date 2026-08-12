package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

// VerifyCallOutcome revalidates a durable result with every exact out-of-line
// evidence body. Accepted results additionally rederive the typed decision
// against the snapshot that authorized the call.
func VerifyCallOutcome(
	snapshot cognition.RuntimeSnapshot,
	attempt CallAttempt,
	result CallResult,
	evidence CallEvidence,
) (*cognition.CognitionDecision, error) {
	if err := snapshot.Validate(); err != nil || snapshot.SHA256() != attempt.SnapshotSHA256 {
		return nil, fmt.Errorf("%w: call outcome snapshot authority changed: %v", ErrInvalidEvidence, err)
	}
	if err := result.Validate(attempt); err != nil {
		return nil, err
	}
	if err := evidence.ValidateFor(attempt, result); err != nil {
		return nil, err
	}
	switch result.Status {
	case CallResultAccepted:
		decision, err := decisionFromAcceptedResult(result, evidence.Response, snapshot)
		if err != nil {
			return nil, err
		}
		return &decision, nil
	case CallResultRejected, CallResultFailed:
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: call outcome status is not registered", ErrInvalidEvidence)
	}
}
