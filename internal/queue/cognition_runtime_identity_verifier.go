package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

// VerifyCognitionTraceActionIdentity reuses the durable action ID derivation.
func VerifyCognitionTraceActionIdentity(action CognitionTraceAction) error {
	if err := action.Validate(); err != nil {
		return err
	}
	want := cognitionActionID(action.EpisodeID, action.ExpectedRevision, action.PolicyCallID)
	if action.RegisteredAction.ID != want {
		return fmt.Errorf("%w: cognition action identity changed", ErrCognitionConflict)
	}
	return nil
}

// VerifyCognitionTraceTransitionIdentity reuses the durable transition ID
// derivation after its exact canonical payload digest has been verified.
func VerifyCognitionTraceTransitionIdentity(
	episodeID cognition.EpisodeID,
	recordID string,
	transitionSHA256 string,
) error {
	if recordID != cognitionTransitionID(episodeID, transitionSHA256) {
		return fmt.Errorf("%w: cognition transition identity changed", ErrCognitionConflict)
	}
	return nil
}
