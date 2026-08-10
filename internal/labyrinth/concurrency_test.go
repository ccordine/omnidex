package labyrinth

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestConcurrentExactRetriesReturnOneImmutableTransition(t *testing.T) {
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	action := world.action(t, "action-concurrent-replay", "enable", "unit-public")
	const callers = 32
	results := make(chan cognition.Transition, callers)
	errors := make(chan error, callers)
	var group sync.WaitGroup
	for index := 0; index < callers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			transition, applyErr := environment.Apply(context.Background(), world.episode, started.Current, action)
			results <- transition
			errors <- applyErr
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent replay: %v", err)
		}
	}
	var first cognition.Transition
	for transition := range results {
		if first.Current.Number == 0 {
			first = transition
			continue
		}
		if !reflect.DeepEqual(first, transition) {
			t.Fatalf("concurrent retries returned different transitions:\nfirst=%#v\nnext=%#v", first, transition)
		}
	}
	if first.Current.Number != 2 {
		t.Fatalf("concurrent retries advanced more than once: %#v", first)
	}
}

func TestAuthorizedReplacementActorCanReplaySameSemanticAction(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	replacement := world.actor
	replacement.Attempt++
	replacement.WorkerID = "worker-test-2"
	environment, err := NewEnvironment(world.kernel, world.episode, func(_ context.Context, actor cognition.AttemptRef) error {
		if actor != world.actor && actor != replacement {
			return fmt.Errorf("%w: actor is not current", cognition.ErrAuthorityDenied)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	action := world.action(t, "action-replacement-replay", "enable", "unit-public")
	first, err := environment.Apply(context.Background(), world.episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	action.Actor = replacement
	replayed, err := environment.Apply(context.Background(), world.episode, started.Current, action)
	if err != nil {
		t.Fatalf("replacement replay: %v", err)
	}
	if !reflect.DeepEqual(first, replayed) {
		t.Fatalf("actor fence changed semantic replay identity:\nfirst=%#v\nreplay=%#v", first, replayed)
	}
}
