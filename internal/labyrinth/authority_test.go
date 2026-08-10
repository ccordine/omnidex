package labyrinth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestApplyEffectsAndFailuresUseExactCognitionAuthority(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	if len(started.Effects) != 0 {
		t.Fatalf("start reported action effects: %#v", started.Effects)
	}
	precondition := world.action(t, "action-typed-failure", "disable", "unit-public")
	_, err = environment.Apply(context.Background(), world.episode, started.Current, precondition)
	var failure cognition.ActionFailure
	if !errors.As(err, &failure) || !errors.Is(err, cognition.ErrActionFailed) || !errors.Is(err, ErrPrecondition) {
		t.Fatalf("precondition failure is not fully typed: %v", err)
	}
	if failure.Code != cognition.ActionFailurePreconditionFailed || failure.ActionID != precondition.ID || failure.Revision != started.Current {
		t.Fatalf("failure authority is incomplete: %#v", failure)
	}

	action := world.action(t, "action-typed-effect", "enable", "unit-public")
	transition, err := environment.Apply(context.Background(), world.episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	if len(transition.Effects) != 1 {
		t.Fatalf("effect count = %d, want 1", len(transition.Effects))
	}
	effect := transition.Effects[0]
	if err := effect.Validate(); err != nil {
		t.Fatalf("validate effect: %v", err)
	}
	if effect.ActionID != action.ID || effect.Revision != transition.Current || effect.Kind != cognition.EffectStateChanged {
		t.Fatalf("effect authority is incomplete: %#v", effect)
	}
	if strings.Contains(effect.Content, hiddenStateCanary) {
		t.Fatalf("public effect leaks hidden state: %#v", effect)
	}
}

func TestExactRevisionAndEpisodeFencesRejectWithoutMutation(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	wrongHash := started.Current
	wrongHash.SHA256 = strings.Repeat("a", 64)
	if wrongHash == started.Current {
		wrongHash.SHA256 = strings.Repeat("b", 64)
	}
	if _, err := environment.Apply(
		context.Background(), world.episode, wrongHash,
		world.action(t, "action-wrong-hash", "enable", "unit-public"),
	); !errors.Is(err, cognition.ErrInvalidRevision) {
		t.Fatalf("wrong hash error = %v, want ErrInvalidRevision", err)
	}
	foreignEpisode := cognition.EpisodeRef{ID: "episode-foreign"}
	if _, err := environment.Apply(
		context.Background(), foreignEpisode, started.Current,
		world.action(t, "action-foreign-episode", "enable", "unit-public"),
	); !errors.Is(err, cognition.ErrInvalidRevision) {
		t.Fatalf("foreign episode error = %v, want ErrInvalidRevision", err)
	}
	transition, err := environment.Apply(
		context.Background(), world.episode, started.Current,
		world.action(t, "action-after-fences", "enable", "unit-public"),
	)
	if err != nil {
		t.Fatalf("valid action after rejected fences: %v", err)
	}
	if transition.Current.Number != 2 {
		t.Fatalf("rejected fence advanced state: %#v", transition)
	}
}

func TestEnvironmentRejectsEvidenceItDidNotProduce(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	fake := cognition.EvidenceRef{
		ObservationID: "observation-not-produced",
		Revision:      started.Current,
		SHA256:        strings.Repeat("a", 64),
	}
	action, err := cognition.NewRegisteredAction(
		"action-fake-evidence",
		world.actor,
		world.schemas["enable"],
		cognition.ActionRequest{
			Kind: "enable", Arguments: []cognition.ActionArgument{{Name: "target", Value: "unit-public"}},
		},
		[]cognition.EvidenceRef{fake},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.Apply(context.Background(), world.episode, started.Current, action); !errors.Is(err, cognition.ErrInvalidEvidence) {
		t.Fatalf("unknown evidence error = %v, want ErrInvalidEvidence", err)
	}
	if _, err := environment.Apply(
		context.Background(), world.episode, started.Current,
		world.action(t, "action-after-fake-evidence", "enable", "unit-public"),
	); err != nil {
		t.Fatalf("valid action after rejected evidence: %v", err)
	}
}

func TestEnvironmentRequiresExplicitAttemptAuthority(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	if _, err := NewEnvironment(world.kernel, world.episode, nil); !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("nil authorizer error = %v, want ErrAuthorityDenied", err)
	}
}
