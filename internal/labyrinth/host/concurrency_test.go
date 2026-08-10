package host

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPostgresConcurrentCandidatesCannotBothAdvanceOneRevision(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	started, err := environment.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	actions := []cognition.RegisteredAction{
		fixture.action(t, 0, "concurrent-left", fixture.Actor),
		fixture.action(t, 0, "concurrent-right", fixture.Actor),
	}
	type outcome struct {
		transition cognition.Transition
		err        error
	}
	outcomes := make(chan outcome, len(actions))
	var ready sync.WaitGroup
	ready.Add(len(actions))
	gate := make(chan struct{})
	for _, action := range actions {
		action := action
		go func() {
			ready.Done()
			<-gate
			transition, applyErr := environment.Apply(
				context.Background(), fixture.Episode, started.Current, action,
			)
			outcomes <- outcome{transition: transition, err: applyErr}
		}()
	}
	ready.Wait()
	close(gate)
	successes, stale := 0, 0
	for range actions {
		result := <-outcomes
		if result.err == nil {
			successes++
			if result.transition.Current.Number != 2 {
				t.Fatalf("successful concurrent revision = %d", result.transition.Current.Number)
			}
			continue
		}
		if errors.Is(result.err, cognition.ErrInvalidRevision) {
			stale++
			continue
		}
		t.Fatalf("unexpected concurrent result: %v", result.err)
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent outcomes: successes=%d stale=%d", successes, stale)
	}
	head, err := fixture.Store.Episode(context.Background(), fixture.Episode)
	if err != nil {
		t.Fatal(err)
	}
	if head.Current.Number != 2 {
		t.Fatalf("concurrent durable head revision = %d", head.Current.Number)
	}
}
