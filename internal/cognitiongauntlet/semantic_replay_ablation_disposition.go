package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func ablationTransitionCalls(root ablationEvidenceRoot) map[string]int64 {
	result := make(map[string]int64, len(root.Actions))
	for _, value := range root.Actions {
		if value.Trace.Transition != nil {
			result[string(value.Trace.Transition.ActionID)] = int64(value.Cycle)
		}
	}
	return result
}

func ablationDispositionUnits(root ablationEvidenceRoot) []ablationSemanticUnit {
	values := make([]ablationSemanticUnit, 0, len(root.Actions)+len(root.NoActions))
	for _, action := range root.Actions {
		events := []cognitionreplay.EventKind{cognitionreplay.EventActionSelected}
		if action.Trace.Failure != nil {
			events = append(events, cognitionreplay.EventFailureRecorded)
		}
		values = append(values, ablationUnit(
			int64(action.Cycle), 30, 0, "ablation.action_outcome",
			string(action.Trace.Action.ID), action, events...,
		))
	}
	for _, disposition := range root.NoActions {
		values = append(values, ablationUnit(
			int64(disposition.Cycle), 30, 0, "ablation.call_disposition",
			fmt.Sprintf("%s-disposition", disposition.CallID), disposition,
			cognitionreplay.EventModelCallDisposition, cognitionreplay.EventFailureRecorded,
		))
	}
	return values
}
