package cognition

import (
	"errors"
	"testing"
)

func TestStartTransitionHasNoActionAuthority(t *testing.T) {
	t.Parallel()
	observation, err := NewObservation("observation-1", testRevision(1), "state", "initial")
	if err != nil {
		t.Fatal(err)
	}
	transition := Transition{Current: testRevision(1), Observations: []Observation{observation}}
	if err := transition.ValidateStart(); err != nil {
		t.Fatalf("validate start: %v", err)
	}

	for name, mutate := range map[string]func(*Transition){
		"previous revision": func(value *Transition) { revision := testRevision(1); value.Previous = &revision },
		"action identity":   func(value *Transition) { value.ActionID = "action-1" },
		"noninitial number": func(value *Transition) { value.Current.Number = 2 },
		"nonzero cost":      func(value *Transition) { value.Cost = 1 },
		"action effect": func(value *Transition) {
			effect, err := NewEffect("action-1", value.Current, EffectStateChanged, "changed")
			if err != nil {
				t.Fatal(err)
			}
			value.Effects = []Effect{effect}
		},
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := transition
			mutate(&candidate)
			if err := candidate.ValidateStart(); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want ErrInvalidTransition", err)
			}
		})
	}
}

func TestApplyTransitionBindsExpectedRevisionAndActionIdentity(t *testing.T) {
	t.Parallel()
	schema := testActionSchema(t, EvidenceRequired)
	action, err := NewRegisteredAction(
		"action-1",
		testAttemptRef(),
		schema,
		ActionRequest{Kind: "inspect", Arguments: []ActionArgument{{Name: "target", Value: "entity-1"}}},
		[]EvidenceRef{testEvidenceRef(t)},
	)
	if err != nil {
		t.Fatal(err)
	}
	previous := testRevision(1)
	current := testRevision(2)
	observation, err := NewActionObservation("observation-2", action.ID, current, "state", "updated")
	if err != nil {
		t.Fatal(err)
	}
	effect, err := NewEffect(action.ID, current, EffectObservationProduced, "Exposed updated public state.")
	if err != nil {
		t.Fatal(err)
	}
	transition := Transition{
		ActionID: action.ID, Previous: &previous, Current: current,
		Observations: []Observation{observation}, Effects: []Effect{effect}, Cost: 1,
	}
	episode := EpisodeRef{ID: "episode-1"}
	if err := transition.ValidateApply(episode, previous, action); err != nil {
		t.Fatalf("validate apply: %v", err)
	}

	for name, mutate := range map[string]func(*Transition){
		"missing previous":   func(value *Transition) { value.Previous = nil },
		"stale previous":     func(value *Transition) { value.Previous.Number = 2 },
		"wrong action":       func(value *Transition) { value.ActionID = "action-2" },
		"skipped revision":   func(value *Transition) { value.Current.Number = 3 },
		"different episode":  func(value *Transition) { value.Current.EpisodeID = "episode-2" },
		"negative cost":      func(value *Transition) { value.Cost = -1 },
		"observation stale":  func(value *Transition) { value.Observations[0].Revision.Number = 1 },
		"observation action": func(value *Transition) { value.Observations[0].ActionID = "action-2" },
		"effect action":      func(value *Transition) { value.Effects[0].ActionID = "action-2" },
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := transition.Clone()
			mutate(&candidate)
			if err := candidate.ValidateApply(episode, previous, action); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want ErrInvalidTransition", err)
			}
		})
	}
}

func TestTransitionRejectsDuplicateAndExcessObservations(t *testing.T) {
	t.Parallel()
	current := testRevision(1)
	observation, err := NewObservation("observation-1", current, "state", "initial")
	if err != nil {
		t.Fatal(err)
	}
	duplicate := Transition{Current: current, Observations: []Observation{observation, observation}}
	if err := duplicate.ValidateStart(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("duplicate observation error = %v, want ErrInvalidTransition", err)
	}
	overflow := Transition{Current: current, Observations: make([]Observation, MaxTransitionObservations+1)}
	if err := overflow.ValidateStart(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("observation overflow error = %v, want ErrInvalidTransition", err)
	}
}
