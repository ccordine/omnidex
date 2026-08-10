package cognitiongauntlet

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

const ActionTraceSchemaV1 = "omnidex.cognition-action-trace.v1"

type ActionTrace struct {
	Schema           string                     `json:"schema"`
	Action           cognition.RegisteredAction `json:"action"`
	ExpectedRevision cognition.WorldRevision    `json:"expected_revision"`
	Transition       *cognition.Transition      `json:"transition,omitempty"`
	Failure          *cognition.ActionFailure   `json:"failure,omitempty"`
}

type oracleTerminalTrace struct {
	Revision      cognition.WorldRevision `json:"revision"`
	PublicOutcome string                  `json:"public_outcome"`
	GoalSatisfied bool                    `json:"goal_satisfied"`
}

func appendTransitionObservations(
	recorder *EpisodeRecorder,
	transition cognition.Transition,
) error {
	for _, observation := range transition.Observations {
		payload, err := traceJSONObject(observation)
		if err != nil {
			return err
		}
		if err := recorder.Append(
			TraceObservation, string(observation.ID), &transition.Current, payload,
		); err != nil {
			return err
		}
	}
	return nil
}

func appendOracleAction(
	recorder *EpisodeRecorder,
	action cognition.RegisteredAction,
	transition cognition.Transition,
) error {
	if transition.Previous == nil {
		return fmt.Errorf("oracle action transition lacks its expected revision")
	}
	copy := transition.Clone()
	payload, err := traceJSONObject(ActionTrace{
		Schema: ActionTraceSchemaV1, Action: action.Clone(),
		ExpectedRevision: *transition.Previous, Transition: &copy,
	})
	if err != nil {
		return err
	}
	return recorder.Append(TraceAction, string(action.ID), &transition.Current, payload)
}

func appendOracleTerminal(recorder *EpisodeRecorder, transition cognition.Transition) error {
	payload, err := traceJSONObject(oracleTerminalTrace{
		Revision: transition.Current, PublicOutcome: transition.PublicOutcome,
		GoalSatisfied: transition.Terminal,
	})
	if err != nil {
		return err
	}
	return recorder.Append(TraceTerminal, "terminal-"+transition.Current.SHA256, &transition.Current, payload)
}

func traceJSONObject(value any) (taskstate.JSONObject, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return taskstate.JSONObject{}, fmt.Errorf("encode cognition gauntlet trace payload: %w", err)
	}
	payload, err := taskstate.NewJSONObject(raw)
	if err != nil {
		return taskstate.JSONObject{}, fmt.Errorf("validate cognition gauntlet trace payload: %w", err)
	}
	return payload, nil
}
