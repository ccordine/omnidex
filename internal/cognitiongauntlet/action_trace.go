package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func decodeActionTrace(entry TraceEntry, episode cognition.EpisodeRef) (ActionTrace, error) {
	trace := ActionTrace{}
	if err := decodeTracePayload(entry.Payload, &trace, "registered action trace"); err != nil {
		return ActionTrace{}, err
	}
	if trace.Schema != ActionTraceSchemaV1 || string(trace.Action.ID) != entry.ID ||
		entry.Revision == nil || trace.ExpectedRevision.EpisodeID != episode.ID ||
		trace.ExpectedRevision.Validate() != nil || (trace.Transition == nil) == (trace.Failure == nil) {
		return ActionTrace{}, fmt.Errorf("registered action trace authority is invalid")
	}
	if trace.Transition != nil {
		if trace.Transition.Previous == nil || *trace.Transition.Previous != trace.ExpectedRevision ||
			trace.Transition.Current != *entry.Revision {
			return ActionTrace{}, fmt.Errorf("registered action trace transition authority is invalid")
		}
		if err := trace.Transition.ValidateApply(episode, trace.ExpectedRevision, trace.Action); err != nil {
			return ActionTrace{}, fmt.Errorf("registered action trace transition: %w", err)
		}
		return trace, nil
	}
	if *entry.Revision != trace.ExpectedRevision || trace.Failure == nil {
		return ActionTrace{}, fmt.Errorf("registered action failure revision is invalid")
	}
	if err := trace.Failure.Validate(trace.Action, trace.ExpectedRevision); err != nil {
		return ActionTrace{}, fmt.Errorf("registered action trace failure: %w", err)
	}
	return trace, nil
}
