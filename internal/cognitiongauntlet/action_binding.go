package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

type tracedActionBinding struct {
	Sequence     uint64
	Action       cognition.RegisteredAction
	ProjectionID string
}

func privateWitnessAction(
	oracle labyrinth.Oracle,
	id cognition.ActionID,
) (labyrinth.WitnessAction, error) {
	return privateWitnessActionIn(oracle.Witness, id)
}

func privateWitnessActionIn(
	witness []labyrinth.WitnessAction,
	id cognition.ActionID,
) (labyrinth.WitnessAction, error) {
	for _, action := range witness {
		if action.ID == id {
			return action, nil
		}
	}
	return labyrinth.WitnessAction{}, fmt.Errorf("private evidence use cites an absent witness action")
}

func tracedActionBindings(episode SealedEpisode) ([]tracedActionBinding, error) {
	bindings := make([]tracedActionBinding, 0, episode.Manifest.Resources.EnvironmentActions)
	latestProjection := ""
	for _, entry := range episode.Manifest.Trace {
		switch entry.Kind {
		case TraceModelCall:
			call := ModelCallTrace{}
			if err := decodeTracePayload(entry.Payload, &call, "action-binding model call"); err != nil {
				return nil, err
			}
			latestProjection = call.ProjectionID
		case TraceAction:
			trace, err := decodeActionTrace(entry, cognition.EpisodeRef{ID: episode.Manifest.EpisodeID})
			if err != nil {
				return nil, err
			}
			if trace.Transition == nil {
				latestProjection = ""
				continue
			}
			bindings = append(bindings, tracedActionBinding{
				Sequence: entry.Sequence, Action: trace.Action.Clone(), ProjectionID: latestProjection,
			})
			latestProjection = ""
		}
	}
	return bindings, nil
}

func uniqueActionForRequest(
	bindings []tracedActionBinding,
	request cognition.ActionRequest,
) (tracedActionBinding, error) {
	matched, found, err := optionalActionForRequest(bindings, request)
	if err != nil {
		return tracedActionBinding{}, err
	}
	if !found {
		return tracedActionBinding{}, fmt.Errorf(
			"private witness request has 0 exact accepted-action matches",
		)
	}
	return matched, nil
}

func optionalActionForRequest(
	bindings []tracedActionBinding,
	request cognition.ActionRequest,
) (tracedActionBinding, bool, error) {
	want, err := digestJSON(request)
	if err != nil {
		return tracedActionBinding{}, false, fmt.Errorf("hash private witness request: %w", err)
	}
	var matched tracedActionBinding
	matches := 0
	for _, binding := range bindings {
		actual, err := digestJSON(binding.Action.Request)
		if err != nil {
			return tracedActionBinding{}, false, fmt.Errorf("hash sealed action request: %w", err)
		}
		if actual == want {
			matched = binding
			matches++
		}
	}
	if matches > 1 {
		return tracedActionBinding{}, false, fmt.Errorf(
			"private witness request has %d exact accepted-action matches", matches,
		)
	}
	return matched, matches == 1, nil
}
