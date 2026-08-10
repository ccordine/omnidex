package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *productionTraceState) acceptAction(
	recorder *EpisodeRecorder,
	record queue.CognitionSealedTraceRecord,
) error {
	var action queue.CognitionTraceAction
	if err := decodeProductionPayload(record.Payload, &action, "registered action"); err != nil {
		return err
	}
	if err := action.Validate(); err != nil {
		return err
	}
	if action.EpisodeID != state.trace.Header.EpisodeID ||
		action.PolicyCallID == "" || action.ContextProjection.ID == "" {
		return fmt.Errorf("sealed production action changed episode or call authority")
	}
	if _, duplicate := state.actions[action.RegisteredAction.ID]; duplicate {
		return fmt.Errorf("sealed production action identity is duplicated")
	}
	state.actions[action.RegisteredAction.ID] = action
	resources := &state.metrics.Resources
	resources.EnvironmentActions++
	resources.ToolOperations++
	switch action.RegisteredAction.Request.Kind {
	case "search":
		resources.SearchOperations++
	case "read":
		resources.ReadOperations++
	}
	if action.Status != queue.CognitionActionFailed {
		return nil
	}
	if action.Failure == nil {
		return fmt.Errorf("failed sealed production action omitted its typed failure")
	}
	trace := ActionTrace{
		Schema: ActionTraceSchemaV1, Action: action.RegisteredAction.Clone(),
		ExpectedRevision: action.ExpectedRevision,
	}
	failure := action.Failure.Clone()
	trace.Failure = &failure
	if err := appendProductionActionTrace(recorder, trace, action.ExpectedRevision); err != nil {
		return err
	}
	state.consumedActions[action.RegisteredAction.ID] = struct{}{}
	state.metrics.Planning.InvalidActions++
	return nil
}

func (state *productionTraceState) acceptTransition(
	recorder *EpisodeRecorder,
	record queue.CognitionSealedTraceRecord,
) error {
	var transition cognition.Transition
	if err := decodeProductionPayload(record.Payload, &transition, "environment transition"); err != nil {
		return err
	}
	if transition.ActionID == "" {
		if err := transition.ValidateStart(); err != nil {
			return err
		}
		if transition.Current.EpisodeID != state.trace.Header.EpisodeID {
			return fmt.Errorf("sealed production start transition belongs to another episode")
		}
		return appendTransitionObservations(recorder, transition)
	}
	action, exists := state.actions[transition.ActionID]
	if !exists || action.Status != queue.CognitionActionSucceeded || action.ResultRevision == nil {
		return fmt.Errorf("sealed production transition has no exact successful action authority")
	}
	if *action.ResultRevision != transition.Current || transition.Previous == nil ||
		*transition.Previous != action.ExpectedRevision {
		return fmt.Errorf("sealed production transition changed the action revision authority")
	}
	copy := transition.Clone()
	actionTrace := ActionTrace{
		Schema: ActionTraceSchemaV1, Action: action.RegisteredAction.Clone(),
		ExpectedRevision: action.ExpectedRevision, Transition: &copy,
	}
	if err := appendProductionActionTrace(recorder, actionTrace, transition.Current); err != nil {
		return err
	}
	if err := appendTransitionObservations(recorder, transition); err != nil {
		return err
	}
	state.consumedActions[transition.ActionID] = struct{}{}
	state.metrics.Resources.LowLevelTransitions++
	return nil
}

func appendProductionActionTrace(
	recorder *EpisodeRecorder,
	action ActionTrace,
	revision cognition.WorldRevision,
) error {
	payload, err := traceJSONObject(action)
	if err != nil {
		return err
	}
	return recorder.Append(TraceAction, string(action.Action.ID), &revision, payload)
}
