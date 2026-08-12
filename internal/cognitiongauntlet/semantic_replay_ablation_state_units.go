package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/workingset"
)

func ablationStateUnits(
	root ablationEvidenceRoot,
	transitionCalls map[string]int64,
) ([]ablationSemanticUnit, error) {
	observationCalls := make([]int64, 0)
	for index, transition := range root.Transitions {
		call := transitionCalls[string(transition.ActionID)]
		if index == 0 {
			call = 0
		}
		for range transition.Observations {
			observationCalls = append(observationCalls, call)
		}
	}
	values := []ablationSemanticUnit{}
	if root.Ledger != nil {
		for index, event := range root.Ledger.Events {
			call := int64(0)
			if index < len(observationCalls) {
				call = observationCalls[index]
			}
			values = append(values, ablationUnit(
				call, 60, int64(event.Version), "ablation.ledger_event",
				fmt.Sprintf("ledger-%s-event-%d", event.LedgerID, event.Version), event,
				cognitionreplay.EventEvidenceAcquired,
			))
		}
	}
	if root.WorkingSet == nil {
		return values, nil
	}
	values = append(values, ablationUnit(
		0, 5, 0, "ablation.working_set_initial",
		"working-set-"+string(root.WorkingSet.Initial.ID)+"-initial",
		root.WorkingSet.Initial, cognitionreplay.EventWorkingSetSnapshot,
	))
	set, err := workingset.Restore(root.WorkingSet.Initial)
	if err != nil {
		return nil, fmt.Errorf("restore ablation semantic Working Set: %w", err)
	}
	for index, event := range root.WorkingSet.Events {
		before := workingSetItems(set.Items())
		command, decodeErr := workingset.DecodeCommand(event.CommandKind, event.Command)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode ablation semantic Working Set event: %w", decodeErr)
		}
		if _, applyErr := set.Apply(command); applyErr != nil {
			return nil, fmt.Errorf("replay ablation semantic Working Set event: %w", applyErr)
		}
		after := workingSetItems(set.Items())
		dummy := cognitionreplay.SourceRecord{
			Ordinal: 1, CallOrdinal: 0, Phase: 1, Sequence: 0,
			Kind: "ablation.working_set_event", ID: "working-set-diff",
		}
		drafts, draftErr := semanticWorkingSetDrafts(dummy, command, before, after)
		if draftErr != nil {
			return nil, draftErr
		}
		events := make([]cognitionreplay.EventKind, len(drafts))
		for draftIndex, draft := range drafts {
			events[draftIndex] = draft.Kind
		}
		call := int64(0)
		if index < len(observationCalls) {
			call = observationCalls[index]
		}
		unit := ablationUnit(
			call, 70, int64(event.Version), "ablation.working_set_event",
			fmt.Sprintf("working-set-%s-event-%d", event.SetID, event.Version), event, events...,
		)
		unit.knowledge = make([]*semanticKnowledgeChange, len(drafts))
		for draftIndex, draft := range drafts {
			unit.knowledge[draftIndex] = draft.Knowledge
		}
		values = append(values, unit)
	}
	return values, nil
}
