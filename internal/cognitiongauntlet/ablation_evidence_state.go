package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func ablationReplayClass(variant Variant) (AblationReplayClass, bool, error) {
	switch variant {
	case VariantRawObservation, VariantFullTranscript, VariantTranscriptCompacted,
		VariantTaskLedger, VariantLedgerWorkingSet, VariantLedgerProjection:
		return AblationReplaySerious, false, nil
	case VariantRawShell:
		return AblationReplayBenchmarkOnly, false, nil
	case VariantOracleEvidence:
		return AblationReplayContaminated, true, nil
	default:
		return "", false, fmt.Errorf("ablation replay variant is not executable")
	}
}

func freezeAblationLedger(state *ablationState) (*ablationLedgerEvidence, error) {
	if state.ledger == nil {
		if ledgerBackedAblation(state.variant) {
			return nil, fmt.Errorf("ledger-backed ablation lacks its Task Ledger")
		}
		return nil, nil
	}
	if !ledgerBackedAblation(state.variant) {
		return nil, fmt.Errorf("non-ledger ablation contains a Task Ledger")
	}
	events := state.ledger.Events()
	terminal := state.ledger.MaterializedState()
	reconstructed, err := taskstate.Reconstruct(state.ledger.ID(), state.ledger.Owner(), events)
	if err != nil || !reflect.DeepEqual(reconstructed.MaterializedState(), terminal) {
		return nil, fmt.Errorf("ablation Task Ledger history differs from terminal state: %v", err)
	}
	return &ablationLedgerEvidence{
		ID: state.ledger.ID(), Owner: state.ledger.Owner(), Events: events, Terminal: terminal,
	}, nil
}

func freezeAblationWorkingSet(state *ablationState) (*ablationWorkingSetEvidence, error) {
	required := state.variant == VariantLedgerWorkingSet || state.variant == VariantLedgerProjection
	if state.workingSet == nil {
		if required {
			return nil, fmt.Errorf("Working-Set ablation lacks its exact state")
		}
		return nil, nil
	}
	if !required || state.workingSetStart == nil {
		return nil, fmt.Errorf("ablation contains an unexpected Working Set")
	}
	initial := *state.workingSetStart
	terminal := state.workingSet.Snapshot()
	reconstructed, err := workingset.Restore(initial)
	if err != nil {
		return nil, fmt.Errorf("restore ablation Working Set: %w", err)
	}
	for index, supplied := range state.workingSetEvents {
		command, err := workingset.DecodeCommand(supplied.CommandKind, supplied.Command)
		if err != nil {
			return nil, fmt.Errorf("decode ablation Working Set event %d: %w", index+1, err)
		}
		actual, err := reconstructed.Apply(command)
		if err != nil || !reflect.DeepEqual(actual, supplied) {
			return nil, fmt.Errorf("ablation Working Set event %d differs on replay: %v", index+1, err)
		}
	}
	if !reflect.DeepEqual(reconstructed.Snapshot(), terminal) {
		return nil, fmt.Errorf("ablation Working Set history differs from terminal state")
	}
	events := append([]workingset.Event(nil), state.workingSetEvents...)
	return &ablationWorkingSetEvidence{Initial: initial, Events: events, Terminal: terminal}, nil
}
