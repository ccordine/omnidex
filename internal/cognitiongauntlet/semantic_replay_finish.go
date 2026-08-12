package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) finish(outcome Outcome) (
	[]cognitionreplay.Event,
	[]cognitionreplay.KnowledgeCheckpoint,
	[]cognitionreplay.Blob,
	error,
) {
	if !state.started {
		return nil, nil, nil, fmt.Errorf("semantic replay lacks a world-start event")
	}
	if !state.initialBrainBootstrap || !state.initialProviderObservation {
		return nil, nil, nil, fmt.Errorf(
			"semantic replay lacks exact initial provider bootstrap or process observation",
		)
	}
	if err := state.finishProviderInvocations(); err != nil {
		return nil, nil, nil, err
	}
	if state.workingSet == nil || !state.workingSetTerminal {
		return nil, nil, nil, fmt.Errorf("semantic replay lacks exact Working Set endpoints")
	}
	if !state.terminal {
		return nil, nil, nil, fmt.Errorf("semantic replay lacks exact terminal events")
	}
	if err := state.finishEnvironment(); err != nil {
		return nil, nil, nil, err
	}
	if err := state.finishProgress(); err != nil {
		return nil, nil, nil, err
	}
	if err := state.finishTerminalSealAuthority(); err != nil {
		return nil, nil, nil, err
	}
	if err := state.finishTerminalPublicOutcome(outcome); err != nil {
		return nil, nil, nil, err
	}
	if err := state.finishProviderActivationFailure(); err != nil {
		return nil, nil, nil, err
	}
	if err := state.finishPreparedInputs(); err != nil {
		return nil, nil, nil, err
	}
	if err := state.finishPolicyCalls(); err != nil {
		return nil, nil, nil, err
	}
	if err := state.finishProposalMaterializations(); err != nil {
		return nil, nil, nil, err
	}
	if err := state.finishAcceptedFactMaterializations(); err != nil {
		return nil, nil, nil, err
	}
	if len(state.usedPolicyEvidence) != len(state.evidence.policy) {
		return nil, nil, nil, fmt.Errorf("semantic policy evidence bodies are not exactly consumed")
	}
	if len(state.recoveries) != len(state.recoveryConsumers) {
		return nil, nil, nil, fmt.Errorf("semantic accepted-decision recoveries are not exactly consumed")
	}
	for recoveryID := range state.recoveryConsumers {
		if _, exists := state.recoveries[recoveryID]; !exists {
			return nil, nil, nil, fmt.Errorf("semantic recovery consumer lacks exact recovery evidence")
		}
	}
	if len(state.deferredSources) != len(state.consumedDeferredSources) {
		return nil, nil, nil, fmt.Errorf("semantic deferred sources are not exactly consumed")
	}
	if len(state.activationBootstraps) != len(state.activationFailures) {
		return nil, nil, nil, fmt.Errorf("semantic provider activation failures lack exact bootstrap authority")
	}
	for recordID := range state.activationFailures {
		if _, exists := state.activationBootstraps[recordID]; !exists {
			return nil, nil, nil, fmt.Errorf("semantic provider activation failure lacks its bootstrap trace")
		}
	}
	for ordinal := range state.consumedDeferredSources {
		if _, exists := state.deferredSources[ordinal]; !exists {
			return nil, nil, nil, fmt.Errorf("semantic event consumed an unknown deferred source")
		}
	}
	if len(state.checkpoints) == 1 ||
		state.checkpoints[len(state.checkpoints)-1].AfterEvent != uint64(len(state.events)) {
		state.appendCheckpoint()
	}
	return state.events, state.checkpoints, state.eventBlobs, nil
}

func (state *semanticReplayState) finishTerminalPublicOutcome(outcome Outcome) error {
	switch state.trace.Header.Seal.Outcome {
	case queue.CognitionEpisodeCompleted, queue.CognitionEpisodeFailed:
		if !state.worldTerminal || state.terminalProgress == nil ||
			state.worldPublicOutcome != outcome.PublicOutcome ||
			state.terminalProgress.PublicOutcome != outcome.PublicOutcome {
			return fmt.Errorf(
				"semantic terminal environment, progress, and public episode outcome differ",
			)
		}
		if state.trace.Header.Seal.Outcome == queue.CognitionEpisodeFailed &&
			outcome.FailureCode != string(queue.CognitionEpisodeFailed) {
			return fmt.Errorf("semantic failed episode public failure code differs")
		}
	case queue.CognitionEpisodeCanceled:
		if state.cancellation == nil ||
			state.cancellation.PublicMessage != outcome.PublicOutcome ||
			outcome.FailureCode != string(state.cancellation.Code) {
			return fmt.Errorf("semantic cancellation and public episode outcome differ")
		}
	default:
		return fmt.Errorf("semantic terminal public outcome is not registered")
	}
	return nil
}

func (state *semanticReplayState) appendCheckpoint() {
	previous := state.checkpoints[len(state.checkpoints)-1]
	current := uint64(len(state.events))
	entries := semanticReplaySortedEntries(state.entries)
	revision := semanticReplayLatestRevision(state.events)
	upserts := make([]cognitionreplay.KnowledgeEntry, 0, len(entries))
	for _, entry := range entries {
		if latest := entry.SourceEvents[len(entry.SourceEvents)-1]; latest > previous.AfterEvent {
			upserts = append(upserts, entry)
		}
	}
	state.checkpoints = append(state.checkpoints, cognitionreplay.KnowledgeCheckpoint{
		Sequence: uint64(len(state.checkpoints) + 1), AfterEvent: current,
		State: cognitionreplay.KnowledgeState{
			Schema: cognitionreplay.KnowledgeStateSchemaV1, Revision: revision, Entries: entries,
		},
		Delta: &cognitionreplay.KnowledgeDelta{
			Schema:    cognitionreplay.KnowledgeDeltaSchemaV1,
			FromEvent: previous.AfterEvent + 1, ThroughEvent: current,
			SetRevision: revision, Upserts: upserts, Releases: []cognitionreplay.KnowledgeRelease{},
		},
	})
}

func semanticReplayLatestRevision(events []cognitionreplay.Event) *cognitionreplay.PublicRevision {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Revision != nil {
			value := *events[index].Revision
			return &value
		}
	}
	return nil
}

func semanticReplayProgressEvents(progress cognitionruntime.EpisodeProgress) []cognitionreplay.EventKind {
	switch progress.State {
	case cognitionruntime.ProgressActive:
		return []cognitionreplay.EventKind{cognitionreplay.EventObligationChanged}
	case cognitionruntime.ProgressCompleted:
		return []cognitionreplay.EventKind{cognitionreplay.EventGoalSatisfied, cognitionreplay.EventEpisodeSealed}
	case cognitionruntime.ProgressFailed:
		return []cognitionreplay.EventKind{cognitionreplay.EventGoalFailed, cognitionreplay.EventFailureRecorded, cognitionreplay.EventEpisodeSealed}
	default:
		return nil
	}
}

func semanticReplayTransitionRevision(value cognition.Transition) *cognitionreplay.PublicRevision {
	return semanticReplayRevision(value.Current)
}
