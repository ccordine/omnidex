package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func environmentTransitionReceipt(
	t *testing.T,
	episode cognition.EpisodeRef,
	action CognitionActionRecord,
	terminal bool,
) cognition.EnvironmentReceipt {
	t.Helper()
	digest := "2"
	if terminal {
		digest = "3"
	}
	next, err := cognition.NewWorldRevision(episode.ID, action.ExpectedRevision.Number+1, cognitionTestDigest(digest))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation(
		cognition.ObservationID("environment-result-"+digest), action.Action.ID, next,
		"public_state", "One bounded action result.",
	)
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision), Current: next,
		Observations: []cognition.Observation{observation}, Effects: []cognition.Effect{},
		Cost: 1, Terminal: terminal,
	}
	if terminal {
		transition.PublicOutcome = "The registered terminal evidence was acquired."
	}
	receipt, err := cognition.NewEnvironmentTransitionReceipt(
		episode, action.Action, action.ExpectedRevision, transition,
	)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}
