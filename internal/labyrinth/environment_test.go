package labyrinth

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestEnvironmentStartAndApplyProduceExactConsecutiveTransitions(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := started.ValidateStart(); err != nil {
		t.Fatalf("validate start transition: %v", err)
	}
	if started.Current.Number != 1 || started.ActionID != "" || started.Previous != nil || started.Cost != 0 {
		t.Fatalf("invalid start transition: %#v", started)
	}
	if started.Effects == nil || len(started.Effects) != 0 {
		t.Fatalf("start transition must carry an explicit empty effect array: %#v", started.Effects)
	}
	if len(started.Observations) != 1 || strings.Contains(started.Observations[0].Content, hiddenStateCanary) {
		t.Fatalf("initial observation is missing or leaks hidden state: %#v", started.Observations)
	}

	enable := world.action(t, "action-enable-1", "enable", "unit-public")
	enabled, err := environment.Apply(context.Background(), world.episode, started.Current, enable)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := enabled.ValidateApply(world.episode, started.Current, enable); err != nil {
		t.Fatalf("validate enable transition: %v", err)
	}
	if enabled.Current.Number != 2 || enabled.Cost != 2 || enabled.Terminal {
		t.Fatalf("invalid enable transition: %#v", enabled)
	}
	if len(enabled.Observations) != 1 || enabled.Observations[0].ActionID != enable.ID {
		t.Fatalf("observation is not bound to producing action: %#v", enabled.Observations)
	}

	finish := world.action(t, "action-finish-1", "finish", "unit-public")
	finished, err := environment.Apply(context.Background(), world.episode, enabled.Current, finish)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !finished.Terminal || finished.PublicOutcome != PublicOutcomeGoalSatisfied || finished.Current.Number != 3 {
		t.Fatalf("goal transition did not terminate: %#v", finished)
	}
	if _, err := environment.Apply(
		context.Background(), world.episode, finished.Current,
		world.action(t, "action-after-terminal", "disable", "unit-public"),
	); !errors.Is(err, ErrTerminal) {
		t.Fatalf("terminal mutation error = %v, want ErrTerminal", err)
	}
}

func TestEnvironmentReplayIsIdempotentAndConflictFailsBeforeRevisionCheck(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	action := world.action(t, "action-replay-1", "enable", "unit-public")
	first, err := environment.Apply(context.Background(), world.episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := environment.Apply(context.Background(), world.episode, started.Current, action)
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	if !reflect.DeepEqual(first, replayed) {
		t.Fatalf("replay changed transition:\nfirst=%#v\nreplay=%#v", first, replayed)
	}

	conflict := world.action(t, action.ID, "enable", EntityID(hiddenStateCanary))
	if _, err := environment.Apply(context.Background(), world.episode, started.Current, conflict); !errors.Is(err, ErrReplayConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrReplayConflict", err)
	}
	if _, err := environment.Apply(
		context.Background(), world.episode, started.Current,
		world.action(t, "action-stale-1", "disable", "unit-public"),
	); !errors.Is(err, cognition.ErrInvalidRevision) {
		t.Fatalf("unseen stale action error = %v, want ErrInvalidRevision", err)
	}
}

func TestEnvironmentChecksActorAuthorityBeforeReplay(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	action := world.action(t, "action-authority-1", "enable", "unit-public")
	if _, err := environment.Apply(context.Background(), world.episode, started.Current, action); err != nil {
		t.Fatal(err)
	}
	foreign := action
	foreign.Actor.WorkerID = "worker-foreign"
	if _, err := environment.Apply(context.Background(), world.episode, started.Current, foreign); !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("foreign replay error = %v, want ErrAuthorityDenied", err)
	}
}

func TestInvalidActionsDoNotMutateWorldState(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	invalid := world.action(t, "action-invalid-precondition", "disable", "unit-public")
	if _, err := environment.Apply(context.Background(), world.episode, started.Current, invalid); !errors.Is(err, ErrPrecondition) {
		t.Fatalf("precondition error = %v, want ErrPrecondition", err)
	}
	valid := world.action(t, "action-after-invalid", "enable", "unit-public")
	transition, err := environment.Apply(context.Background(), world.episode, started.Current, valid)
	if err != nil {
		t.Fatalf("valid action after failure: %v", err)
	}
	if transition.Previous == nil || *transition.Previous != started.Current || transition.Current.Number != 2 {
		t.Fatalf("failed action advanced state: %#v", transition)
	}
}

func TestFailedActionIdentityCannotLaterChangeMeaning(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	failed := world.action(t, "action-failed-replay", "disable", "unit-public")
	_, firstErr := environment.Apply(context.Background(), world.episode, started.Current, failed)
	var firstFailure cognition.ActionFailure
	if !errors.As(firstErr, &firstFailure) || firstFailure.Code != cognition.ActionFailurePreconditionFailed {
		t.Fatalf("first failure = %v, want typed precondition failure", firstErr)
	}
	enabled, err := environment.Apply(
		context.Background(), world.episode, started.Current,
		world.action(t, "action-enable-before-failed-replay", "enable", "unit-public"),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, replayErr := environment.Apply(context.Background(), world.episode, started.Current, failed)
	var replayFailure cognition.ActionFailure
	if !errors.As(replayErr, &replayFailure) || !reflect.DeepEqual(firstFailure, replayFailure) {
		t.Fatalf("failed replay changed result:\nfirst=%#v\nreplay=%#v\nerror=%v", firstFailure, replayFailure, replayErr)
	}
	conflict := world.action(t, failed.ID, "enable", "unit-public")
	if _, err := environment.Apply(context.Background(), world.episode, enabled.Current, conflict); !errors.Is(err, ErrReplayConflict) {
		t.Fatalf("failed-ID conflict error = %v, want ErrReplayConflict", err)
	}
}

func TestStartRejectsWrongAuthorityWithoutPoisoningEnvironment(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	wrong := world.scenario
	wrong.SHA256 = strings.Repeat("a", 64)
	if wrong == world.scenario {
		wrong.SHA256 = strings.Repeat("b", 64)
	}
	if _, err := environment.Start(context.Background(), wrong); !errors.Is(err, cognition.ErrInvalidScenario) {
		t.Fatalf("wrong scenario error = %v, want ErrInvalidScenario", err)
	}
	if _, err := environment.Start(context.Background(), world.scenario); err != nil {
		t.Fatalf("correct start after rejection: %v", err)
	}
	if _, err := environment.Start(context.Background(), world.scenario); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second start error = %v, want ErrAlreadyStarted", err)
	}
}

func TestReturnedReplayCannotMutateStoredReceipt(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	action := world.action(t, "action-clone-1", "enable", "unit-public")
	first, err := environment.Apply(context.Background(), world.episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	first.Observations[0].Content = "caller mutation"
	first.Previous.Number = 99
	replayed, err := environment.Apply(context.Background(), world.episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	replayedJSON, err := json.Marshal(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if string(replayedJSON) != string(encoded) {
		t.Fatalf("stored replay receipt was caller-mutable:\nwant=%s\ngot=%s", encoded, replayedJSON)
	}
}
