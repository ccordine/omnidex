package cognitiontransport

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

type transportWorld struct {
	scenario    cognition.ScenarioRef
	episode     cognition.EpisodeRef
	action      cognition.RegisteredAction
	start       cognition.Transition
	apply       cognition.Transition
	environment *testEnvironment
}

func newTransportWorld(t *testing.T) transportWorld {
	t.Helper()
	scenario, err := cognition.NewScenarioRef("scenario-test", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	episode, err := cognition.NewEpisodeRef("episode-test")
	if err != nil {
		t.Fatal(err)
	}
	first, err := cognition.NewWorldRevision(episode.ID, 1, strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := cognition.NewObservation("observation-start", first, "public-state", "Initial public state.")
	if err != nil {
		t.Fatal(err)
	}
	start := cognition.Transition{Current: first, Observations: []cognition.Observation{initial}, Effects: []cognition.Effect{}}
	if err := start.ValidateStart(); err != nil {
		t.Fatal(err)
	}
	schema, err := cognition.NewActionSchema("schema-inspect", "1.0.0", "inspect", nil, cognition.EvidenceForbidden)
	if err != nil {
		t.Fatal(err)
	}
	request, err := cognition.NewActionRequest("inspect", nil)
	if err != nil {
		t.Fatal(err)
	}
	action, err := cognition.NewRegisteredAction("action-inspect", cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-1",
	}, schema, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cognition.NewWorldRevision(episode.ID, 2, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation("observation-apply", action.ID, second, "public-state", "Current public state.")
	if err != nil {
		t.Fatal(err)
	}
	effect, err := cognition.NewEffect(action.ID, second, cognition.EffectObservationProduced, "A public observation was produced.")
	if err != nil {
		t.Fatal(err)
	}
	previous := first
	apply := cognition.Transition{
		ActionID: action.ID, Previous: &previous, Current: second,
		Observations: []cognition.Observation{observation}, Effects: []cognition.Effect{effect}, Cost: 1,
	}
	if err := apply.ValidateApply(episode, first, action); err != nil {
		t.Fatal(err)
	}
	environment := &testEnvironment{start: start, apply: apply}
	return transportWorld{scenario, episode, action, start, apply, environment}
}

func mustAuthenticator(t *testing.T, token string) Authenticator {
	t.Helper()
	authenticator, err := NewBearerAuthenticator(token)
	if err != nil {
		t.Fatal(err)
	}
	return authenticator
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
