package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

// VerifyCognitionRuntimeProgressCommandID reuses the durable queue descriptor
// authority to bind a replayed command ID without reading database state.
func VerifyCognitionRuntimeProgressCommandID(
	commandID string,
	command cognitionruntime.CompletionCommand,
	progress cognitionruntime.EpisodeProgress,
) error {
	var kind CognitionObligationCommandKind
	switch progress.State {
	case cognitionruntime.ProgressActive, cognitionruntime.ProgressCompleted:
		kind = CognitionObligationSatisfy
	case cognitionruntime.ProgressFailed:
		kind = CognitionObligationFail
	default:
		return fmt.Errorf("%w: progress state has no durable command kind", ErrCognitionConflict)
	}
	descriptor, err := describeCognitionRuntimeProgress(kind, command)
	if err != nil {
		return err
	}
	if descriptor.ID != commandID {
		return fmt.Errorf("%w: progress command identity changed", ErrCognitionConflict)
	}
	return nil
}
