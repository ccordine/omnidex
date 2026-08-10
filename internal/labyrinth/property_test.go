package labyrinth

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestThousandsOfDeterministicTransitionsPreserveFencesAndReplay(t *testing.T) {
	world := newTestWorld(t)
	left := world.newEnvironment(t)
	right := world.newEnvironment(t)
	leftTransition, err := left.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	rightTransition, err := right.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	const transitions = 3_000
	type historicalTransition struct {
		action     cognition.RegisteredAction
		expected   cognition.WorldRevision
		transition cognition.Transition
	}
	random := rand.New(rand.NewSource(0x51A8E))
	history := make([]historicalTransition, 0, transitions)
	active := false
	for index := 0; index < transitions; index++ {
		validKind, invalidKind := cognition.ActionKind("enable"), cognition.ActionKind("disable")
		if active {
			validKind, invalidKind = invalidKind, validKind
		}
		if random.Intn(2) == 0 {
			invalid := world.action(t, cognition.ActionID(fmt.Sprintf("invalid-%04d", index)), invalidKind, "unit-public")
			if _, applyErr := left.Apply(context.Background(), world.episode, leftTransition.Current, invalid); !errors.Is(applyErr, ErrPrecondition) {
				t.Fatalf("transition %d invalid action error = %v, want ErrPrecondition", index, applyErr)
			}
			if _, applyErr := right.Apply(context.Background(), world.episode, rightTransition.Current, invalid); !errors.Is(applyErr, ErrPrecondition) {
				t.Fatalf("transition %d mirrored invalid action error = %v, want ErrPrecondition", index, applyErr)
			}
		}
		action := world.action(t, cognition.ActionID(fmt.Sprintf("action-%04d", index)), validKind, "unit-public")
		expected := leftTransition.Current
		leftNext, applyErr := left.Apply(context.Background(), world.episode, expected, action)
		if applyErr != nil {
			t.Fatalf("transition %d left apply: %v", index, applyErr)
		}
		rightNext, applyErr := right.Apply(context.Background(), world.episode, rightTransition.Current, action)
		if applyErr != nil {
			t.Fatalf("transition %d right apply: %v", index, applyErr)
		}
		if !reflect.DeepEqual(leftNext, rightNext) {
			t.Fatalf("transition %d is nondeterministic:\nleft=%#v\nright=%#v", index, leftNext, rightNext)
		}
		if leftNext.Current.Number != uint64(index)+2 || leftNext.Previous == nil || *leftNext.Previous != expected {
			t.Fatalf("transition %d broke revision monotonicity: %#v", index, leftNext)
		}
		history = append(history, historicalTransition{action, expected, leftNext.Clone()})
		if random.Intn(7) == 0 {
			selected := history[random.Intn(len(history))]
			replayed, replayErr := left.Apply(context.Background(), world.episode, selected.expected, selected.action)
			if replayErr != nil {
				t.Fatalf("transition %d historical replay: %v", index, replayErr)
			}
			if !reflect.DeepEqual(replayed, selected.transition) {
				t.Fatalf("transition %d historical replay changed receipt", index)
			}
		}
		leftTransition, rightTransition = leftNext, rightNext
		active = !active
	}
}

func TestCancelledOperationsCannotStartOrMutate(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := environment.Start(ctx, world.scenario); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled start error = %v, want context.Canceled", err)
	}
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatalf("start after cancellation: %v", err)
	}
	if _, err := environment.Apply(
		ctx, world.episode, started.Current,
		world.action(t, "action-cancelled", "enable", "unit-public"),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled apply error = %v, want context.Canceled", err)
	}
	transition, err := environment.Apply(
		context.Background(), world.episode, started.Current,
		world.action(t, "action-after-cancel", "enable", "unit-public"),
	)
	if err != nil {
		t.Fatalf("apply after cancellation: %v", err)
	}
	if transition.Current.Number != 2 {
		t.Fatalf("cancelled action advanced revision: %#v", transition)
	}
}
