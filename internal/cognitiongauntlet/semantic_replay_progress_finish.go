package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) finishProgress() error {
	if len(state.progressCommands) != len(state.progressResults) {
		return fmt.Errorf("semantic progress commands and results are not one-to-one")
	}
	for id, command := range state.progressCommands {
		progress, exists := state.progressResults[id]
		graph, graphExists := state.graphs[progress.GraphVersion]
		if !exists || !graphExists || state.graphRecordIDs[progress.GraphVersion] != id ||
			!reflect.DeepEqual(graph, progress.ObligationGraph) ||
			command.sequence != int64(progress.GraphVersion) {
			return fmt.Errorf("semantic progress %q lacks its exact output graph", id)
		}
	}
	if state.trace.Header.GraphVersion == 0 ||
		len(state.graphs) != int(state.trace.Header.GraphVersion) ||
		state.activeGraphVersion != state.trace.Header.GraphVersion {
		return fmt.Errorf("semantic obligation graph stream is incomplete")
	}
	for version := uint64(1); version <= state.trace.Header.GraphVersion; version++ {
		graph, exists := state.graphs[version]
		if !exists {
			return fmt.Errorf("semantic obligation graph version %d is missing", version)
		}
		if version > 1 {
			if err := validateSemanticGraphEvolution(state.graphs[version-1], graph); err != nil {
				return fmt.Errorf("semantic obligation graph version %d: %w", version, err)
			}
		}
		if state.classifiedGraphs[version] == "" {
			return fmt.Errorf("semantic obligation graph version %d lacks exact mutation authority", version)
		}
	}
	for reconciliationID, mutation := range state.graphMutations {
		var matched bool
		for actionID, action := range state.actions {
			if action.ReconciliationID != reconciliationID {
				continue
			}
			transition, transitioned := state.transitions[actionID]
			if matched || action.Status != queue.CognitionActionSucceeded || !transitioned ||
				transition.Terminal || mutation.version < 2 || mutation.kind == "" {
				return fmt.Errorf("semantic graph mutation lacks one successful nonterminal action")
			}
			matched = true
		}
		if !matched {
			return fmt.Errorf("semantic graph mutation lacks its reconciliation action")
		}
	}
	final := state.graphs[state.trace.Header.GraphVersion]
	if final.SHA256 != state.trace.Header.GraphSHA256 {
		return fmt.Errorf("semantic obligation terminal graph differs from the seal")
	}
	graph, err := cognition.RestoreObligationGraph(final)
	if err != nil {
		return err
	}
	terminal, err := graph.TerminalStatus()
	if err != nil {
		return err
	}
	switch state.trace.Header.Seal.Outcome {
	case queue.CognitionEpisodeCompleted:
		if state.terminalProgress == nil || state.cancellation != nil ||
			terminal != cognition.ObligationGraphSatisfied {
			return fmt.Errorf("semantic completed episode lacks one satisfied terminal progress")
		}
	case queue.CognitionEpisodeFailed:
		if state.terminalProgress == nil || state.cancellation != nil ||
			terminal != cognition.ObligationGraphFailed {
			return fmt.Errorf("semantic failed episode lacks one failed terminal progress")
		}
	case queue.CognitionEpisodeCanceled:
		if state.terminalProgress != nil || state.cancellation == nil {
			return fmt.Errorf("semantic canceled episode lacks one cancellation authority")
		}
	default:
		return fmt.Errorf("semantic terminal outcome is not registered")
	}
	if err := state.finishLifecycleRetirement(); err != nil {
		return err
	}
	if state.terminalProgress != nil &&
		(state.terminalProgress.Revision != state.trace.Header.Seal.FinalRevision ||
			state.terminalProgress.GraphVersion != state.trace.Header.GraphVersion ||
			!reflect.DeepEqual(state.terminalProgress.ObligationGraph, final)) {
		return fmt.Errorf("semantic terminal progress differs from the terminal seal")
	}
	return nil
}

func (state *semanticReplayState) finishLifecycleRetirement() error {
	seal := state.trace.Header.Seal
	if state.cancellation == nil {
		if state.lifecycleRetirement != nil || seal.LifecycleOperationID != "" {
			return fmt.Errorf("semantic non-canceled episode contains lifecycle retirement authority")
		}
		return nil
	}
	lifecycle := state.cancellation.Code == cognitionruntime.CancellationJobCanceled ||
		state.cancellation.Code == cognitionruntime.CancellationGenerationRetired
	if !lifecycle {
		if state.lifecycleRetirement != nil || seal.LifecycleOperationID != "" {
			return fmt.Errorf("semantic worker cancellation contains lifecycle retirement authority")
		}
		return nil
	}
	if state.lifecycleRetirement == nil {
		return fmt.Errorf("semantic lifecycle cancellation lacks exact retirement authority")
	}
	if err := queue.VerifyCognitionLifecycleRetirementTraceAuthority(
		*state.lifecycleRetirement, seal, state.trace.Header.GraphVersion,
		state.trace.Header.GraphSHA256, *state.cancellation,
	); err != nil {
		return fmt.Errorf("verify semantic lifecycle retirement: %w", err)
	}
	return nil
}

func validateSemanticGraphEvolution(
	before cognition.ObligationGraphSnapshot,
	after cognition.ObligationGraphSnapshot,
) error {
	if after.Generation < before.Generation || after.Generation > before.Generation+1 {
		return fmt.Errorf("generation does not advance monotonically")
	}
	if after.Generation == before.Generation && after.RootID != before.RootID {
		return fmt.Errorf("root changed without a generation cutover")
	}
	afterItems := make(map[cognition.ObligationID]cognition.Obligation, len(after.Obligations))
	for _, obligation := range after.Obligations {
		afterItems[obligation.ID] = obligation
	}
	for _, prior := range before.Obligations {
		current, exists := afterItems[prior.ID]
		if !exists || current.ParentID != prior.ParentID ||
			!reflect.DeepEqual(current.Desired, prior.Desired) ||
			current.CompletionCheck != prior.CompletionCheck ||
			current.CreatedGeneration != prior.CreatedGeneration ||
			!semanticIDsContained(prior.DependsOn, current.DependsOn) ||
			!semanticEvidenceContained(prior.SupportingRefs, current.SupportingRefs) ||
			!semanticObligationTransition(prior.Status, current.Status) {
			return fmt.Errorf("obligation %q changed immutable or monotonic authority", prior.ID)
		}
		if prior.Completion != nil && !reflect.DeepEqual(prior.Completion, current.Completion) {
			return fmt.Errorf("obligation %q changed terminal completion", prior.ID)
		}
	}
	return nil
}

func semanticObligationTransition(from, to cognition.ObligationStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case cognition.ObligationProposed:
		return to == cognition.ObligationReady || to == cognition.ObligationBlocked ||
			to == cognition.ObligationFailed || to == cognition.ObligationSuperseded
	case cognition.ObligationReady:
		return to == cognition.ObligationActive || to == cognition.ObligationFailed ||
			to == cognition.ObligationSuperseded
	case cognition.ObligationBlocked:
		return to == cognition.ObligationReady || to == cognition.ObligationFailed ||
			to == cognition.ObligationSuperseded
	case cognition.ObligationActive:
		return to == cognition.ObligationBlocked || to == cognition.ObligationSatisfied ||
			to == cognition.ObligationFailed ||
			to == cognition.ObligationSuperseded
	default:
		return false
	}
}

func semanticIDsContained(before, after []cognition.ObligationID) bool {
	available := make(map[cognition.ObligationID]struct{}, len(after))
	for _, id := range after {
		available[id] = struct{}{}
	}
	for _, id := range before {
		if _, exists := available[id]; !exists {
			return false
		}
	}
	return true
}

func semanticEvidenceContained(before, after []cognition.EvidenceRef) bool {
	available := make(map[cognition.EvidenceRef]struct{}, len(after))
	for _, ref := range after {
		available[ref] = struct{}{}
	}
	for _, ref := range before {
		if _, exists := available[ref]; !exists {
			return false
		}
	}
	return true
}
