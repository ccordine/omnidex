package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) finishTerminalSealAuthority() error {
	seal := state.trace.Header.Seal
	switch seal.Outcome {
	case queue.CognitionEpisodeCompleted, queue.CognitionEpisodeFailed:
		if state.terminalProgress == nil || state.terminalProgress.Completion == nil ||
			state.terminalProgressCommandID == "" {
			return fmt.Errorf("semantic worker terminal seal lacks its exact progress completion")
		}
		command, exists := state.progressCommands[state.terminalProgressCommandID]
		if !exists || queue.VerifyCognitionTerminalCompletionTraceAuthority(
			seal, *state.terminalProgress.Completion,
		) != nil || queue.VerifyCognitionWorkerTerminalActorTraceAuthority(
			seal, command.command.Binding.Attempt,
		) != nil {
			return fmt.Errorf("semantic worker terminal seal changed completion or actor authority")
		}
		return nil
	case queue.CognitionEpisodeCanceled:
		completion, err := state.canceledTerminalCompletion()
		if err != nil || queue.VerifyCognitionTerminalCompletionTraceAuthority(
			seal, completion,
		) != nil || state.cancellation == nil {
			return fmt.Errorf("semantic canceled terminal seal changed completion authority: %v", err)
		}
		switch state.cancellation.Code {
		case cognitionruntime.CancellationJobCanceled,
			cognitionruntime.CancellationGenerationRetired:
			// finishLifecycleRetirement binds the lifecycle actor and operation.
			return nil
		case cognitionruntime.CancellationProviderActivation:
			// finishProviderActivationFailure binds the failure receipt actor.
			return nil
		case cognitionruntime.CancellationPolicyFailure,
			cognitionruntime.CancellationRunBudgetExhausted:
			return fmt.Errorf(
				"semantic worker cancellation lacks portable terminal actor authority",
			)
		default:
			return fmt.Errorf("semantic cancellation code has no terminal authority")
		}
	default:
		return fmt.Errorf("semantic terminal seal outcome is not registered")
	}
}

func (state *semanticReplayState) canceledTerminalCompletion() (
	cognition.CompletionResult,
	error,
) {
	graph, exists := state.graphs[state.trace.Header.GraphVersion]
	if !exists {
		return cognition.CompletionResult{}, fmt.Errorf("terminal obligation graph is missing")
	}
	root := cognition.Obligation{}
	for _, obligation := range graph.Obligations {
		if obligation.ID == graph.RootID {
			root = obligation
			break
		}
	}
	if root.ID == "" {
		return cognition.CompletionResult{}, fmt.Errorf("terminal root obligation is missing")
	}
	return cognition.NewCompletionResult(
		root.ID, root.CompletionCheck, state.trace.Header.Seal.FinalRevision,
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
}
