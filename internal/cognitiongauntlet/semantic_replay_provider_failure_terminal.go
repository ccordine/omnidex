package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) finishProviderActivationFailure() error {
	if state.cancellation == nil ||
		state.cancellation.Code != cognitionruntime.CancellationProviderActivation {
		if len(state.activationFailures) != 0 {
			return fmt.Errorf(
				"semantic provider activation failure used another terminal authority",
			)
		}
		return nil
	}
	if len(state.activationFailures) != 1 {
		return fmt.Errorf(
			"semantic provider activation cancellation requires exactly one failure record",
		)
	}
	for _, failure := range state.activationFailures {
		if err := queue.VerifyCognitionProviderActivationFailureTerminalAuthority(
			failure.record, failure.bootstrap, failure.failure, *state.cancellation,
			state.trace.Header.Seal,
		); err != nil {
			return fmt.Errorf("verify semantic provider activation terminal authority: %w", err)
		}
	}
	return nil
}
