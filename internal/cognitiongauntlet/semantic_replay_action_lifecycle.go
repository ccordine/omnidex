package cognitiongauntlet

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type semanticActionEvent struct {
	Schema   string                      `json:"schema"`
	ActionID cognition.ActionID          `json:"action_id"`
	Status   queue.CognitionActionStatus `json:"status"`
	Actor    model.StepAttemptAuthority  `json:"actor"`
	Detail   json.RawMessage             `json:"detail,omitempty"`
}

type semanticActionLifecycle struct {
	prepared   bool
	dispatched bool
	terminal   bool
}

func (state *semanticReplayState) mapActionEvent(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	var value semanticActionEvent
	if err := decodeProductionPayload(record.Payload, &value, "semantic action event"); err != nil ||
		value.Schema != "omnidex.cognition-queue-authority.v1" ||
		!validSemanticActor(value.Actor) ||
		record.ID != string(value.ActionID)+":"+string(value.Status) {
		return nil, fmt.Errorf("invalid semantic action event: %v", err)
	}
	action, exists := state.actions[value.ActionID]
	if !exists || value.Actor != action.Origin ||
		state.actionOrdinals[value.ActionID] != record.CallOrdinal {
		return nil, fmt.Errorf("semantic action event lacks its exact action and actor")
	}
	lifecycle := state.actionEvents[value.ActionID]
	switch value.Status {
	case queue.CognitionActionPrepared:
		if record.Phase != 51 || record.Sequence != 1 || len(value.Detail) != 0 ||
			lifecycle.prepared || lifecycle.dispatched || lifecycle.terminal {
			return nil, fmt.Errorf("semantic prepared action event lifecycle is invalid")
		}
		lifecycle.prepared = true
	case queue.CognitionActionDispatched:
		if record.Phase != 52 || record.Sequence != 2 || len(value.Detail) != 0 ||
			!lifecycle.prepared || lifecycle.dispatched || lifecycle.terminal {
			return nil, fmt.Errorf("semantic dispatched action event lifecycle is invalid")
		}
		lifecycle.dispatched = true
	case queue.CognitionActionSucceeded:
		transition, transitioned := state.transitions[value.ActionID]
		var detail cognition.Transition
		if !semanticTerminalActionEventTuple(value.Status, record.Phase, record.Sequence) || action.Status != value.Status ||
			!lifecycle.prepared || !lifecycle.dispatched || lifecycle.terminal || !transitioned ||
			decodeProductionPayload(value.Detail, &detail, "semantic succeeded action event detail") != nil ||
			!reflect.DeepEqual(detail, transition) {
			return nil, fmt.Errorf("semantic succeeded action event detail or lifecycle is invalid")
		}
		lifecycle.terminal = true
	case queue.CognitionActionFailed:
		var detail cognition.ActionFailure
		if !semanticTerminalActionEventTuple(value.Status, record.Phase, record.Sequence) || action.Status != value.Status ||
			!lifecycle.prepared || !lifecycle.dispatched || lifecycle.terminal || action.Failure == nil ||
			decodeProductionPayload(value.Detail, &detail, "semantic failed action event detail") != nil ||
			!reflect.DeepEqual(detail, *action.Failure) {
			return nil, fmt.Errorf("semantic failed action event detail or lifecycle is invalid")
		}
		if _, transitioned := state.transitions[value.ActionID]; transitioned {
			return nil, fmt.Errorf("failed semantic action also has a transition")
		}
		lifecycle.terminal = true
	default:
		return nil, fmt.Errorf("unregistered action event status %q", value.Status)
	}
	state.actionEvents[value.ActionID] = lifecycle
	return []semanticEventDraft{sourceKnowledgeDraft(
		cognitionreplay.EventEvidenceAcquired, source,
		cognitionreplay.KnowledgeEvidence, cognitionreplay.KnowledgeActive,
		cognitionreplay.AuthorityCode,
	)}, nil
}

func semanticTerminalActionEventTuple(
	status queue.CognitionActionStatus,
	phase int,
	sequence int64,
) bool {
	return (status == queue.CognitionActionSucceeded || status == queue.CognitionActionFailed) &&
		phase == 55 && sequence == 3
}

func (state *semanticReplayState) finishEnvironment() error {
	if state.latestRevision == nil || *state.latestRevision != state.trace.Header.Seal.FinalRevision {
		return fmt.Errorf("semantic world chain does not reach the exact terminal revision")
	}
	for actionID, action := range state.actions {
		lifecycle := state.actionEvents[actionID]
		_, transitioned := state.transitions[actionID]
		if !lifecycle.prepared || !lifecycle.dispatched || !lifecycle.terminal ||
			(action.Status == queue.CognitionActionSucceeded) != transitioned {
			return fmt.Errorf("semantic action %q lacks one exact lifecycle outcome", actionID)
		}
	}
	return nil
}

func validSemanticActor(value model.StepAttemptAuthority) bool {
	if value.Attempt <= 0 {
		return false
	}
	return (cognition.AttemptRef{
		JobID: value.JobID, Generation: value.Generation, StepID: value.StepID,
		Attempt: uint64(value.Attempt), WorkerID: value.WorkerID,
	}).Validate() == nil
}

func validateSemanticProgress(value cognitionruntime.EpisodeProgress, episode cognition.EpisodeID) error {
	if value.Episode.ID != episode || value.Revision.EpisodeID != episode ||
		value.Revision.Validate() != nil || value.GraphVersion == 0 ||
		value.ObligationGraph.Validate() != nil || value.GraphVersion < value.ObligationGraph.Generation {
		return fmt.Errorf("episode progress authority is invalid")
	}
	if value.Completion != nil && value.Completion.Validate() != nil {
		return fmt.Errorf("episode progress completion is invalid")
	}
	if value.PublicOutcome != "" && (value.PublicOutcome != strings.TrimSpace(value.PublicOutcome) || strings.ContainsRune(value.PublicOutcome, 0)) {
		return fmt.Errorf("episode progress public outcome is inexact")
	}
	return nil
}
