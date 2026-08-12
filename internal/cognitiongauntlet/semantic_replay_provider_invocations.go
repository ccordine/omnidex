package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) finishProviderInvocations() error {
	if state.initialBootstrapTrace == nil || len(state.providerProcesses) == 0 {
		return fmt.Errorf("semantic replay lacks its initial provider invocation")
	}
	if len(state.providerProcesses) != len(state.replayBootstraps)+1 {
		return fmt.Errorf("semantic provider bootstrap and process outcomes are not one-to-one")
	}
	initial := *state.initialBootstrapTrace
	process := state.providerProcesses[1]
	bootstrapEvidence, hasBootstrap := state.evidence.identity[initial.Evidence.ID]
	processEvidence, hasProcess := state.evidence.identity[process.Observation.Evidence.ID]
	bootstrap, bootstrapErr := cognitionpolicy.NewBrainBootstrap(initial.Brain, bootstrapEvidence)
	_, activationErr := cognitionpolicy.NewProviderProcessActivation(
		process, processEvidence, initial.Brain,
	)
	if !hasBootstrap || !hasProcess || bootstrapErr != nil || activationErr != nil ||
		!sameFrozenBrain(bootstrap.AttestedBrain, state.frozenBrain) ||
		process.Actor != initial.Actor ||
		process.Observation.ObservedAt.Before(initial.Brain.BootstrapObservation.ObservedAt) {
		return fmt.Errorf("semantic initial provider invocation changed exact raw authority")
	}

	matched := make(map[int64]string, len(state.replayBootstraps))
	for sourceID, replay := range state.replayBootstraps {
		bootstrapEvidence, hasBootstrap = state.evidence.identity[replay.Evidence.ID]
		matches := 0
		for sequence, candidate := range state.providerProcesses {
			if sequence == 1 || candidate.Actor != replay.Actor {
				continue
			}
			candidateEvidence, exists := state.evidence.identity[candidate.Observation.Evidence.ID]
			if !hasBootstrap || !exists ||
				queue.VerifyCognitionEpisodeReplayTraceInvocation(
					replay, bootstrapEvidence, candidate, candidateEvidence,
				) != nil {
				continue
			}
			if prior, used := matched[sequence]; used && prior != sourceID {
				return fmt.Errorf("semantic provider process observation is reused by replay bootstraps")
			}
			matched[sequence] = sourceID
			matches++
		}
		if matches != 1 {
			return fmt.Errorf("semantic replay bootstrap %q lacks one exact process observation", sourceID)
		}
	}
	if len(matched) != len(state.providerProcesses)-1 {
		return fmt.Errorf("semantic provider process observations are not reverse-complete")
	}
	return nil
}

func (state *semanticReplayState) providerProcessAuthority(
	observationID string,
) (cognitionpolicy.ProviderProcessActivationAuthority, error) {
	for _, receipt := range state.providerProcesses {
		if receipt.ID != observationID {
			continue
		}
		evidence, exists := state.evidence.identity[receipt.Observation.Evidence.ID]
		if !exists {
			return cognitionpolicy.ProviderProcessActivationAuthority{},
				fmt.Errorf("semantic provider process observation lacks raw evidence")
		}
		activation, err := cognitionpolicy.NewProviderProcessActivation(
			receipt, evidence, state.frozenBrain,
		)
		if err != nil {
			return cognitionpolicy.ProviderProcessActivationAuthority{}, err
		}
		return activation.Authority()
	}
	return cognitionpolicy.ProviderProcessActivationAuthority{},
		fmt.Errorf("semantic policy call cites an unobserved provider process")
}
