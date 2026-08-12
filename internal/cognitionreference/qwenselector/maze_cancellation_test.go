package qwenselector_test

import (
	"context"
	"errors"
	"testing"
)

func TestMazePreCanceledContextCannotCallWorld(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	world := &cancelingMazeWorld{start: mazeObservation{At: "a", Marker: markerPlain}}
	result, err := runReferenceMaze(ctx, world, &mazeRecordingSelector{selected: "C17"}, 4)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error=%v, want context.Canceled", err)
	}
	if world.starts != 0 || world.moves != 0 || len(result.Moves) != 0 {
		t.Fatalf("pre-canceled run called world or changed state: world=%#v result=%#v", world, result)
	}
}

func TestMazeDiscardsStartObservationWhenContextCancelsDuringStart(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	world := &cancelingMazeWorld{
		cancelStart: cancel,
		start: mazeObservation{
			At: "a", Marker: markerPlain,
			Neighbors: []publicNeighbor{{Cell: "b", Marker: markerPlain}},
		},
	}
	selector := &mazeRecordingSelector{selected: "C17"}
	result, err := runReferenceMaze(ctx, world, selector, 4)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error=%v, want context.Canceled", err)
	}
	if world.moves != 0 || selector.calls != 0 || len(result.Moves) != 0 || result.Complete {
		t.Fatalf("canceled Start changed cognition/world: world=%#v selector=%d result=%#v", world, selector.calls, result)
	}
}

func TestMazeDiscardsMoveObservationWhenContextCancelsDuringMove(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	world := &cancelingMazeWorld{
		cancelMove: cancel,
		start: mazeObservation{
			At: "a", Marker: markerPlain,
			Neighbors: []publicNeighbor{{Cell: "b", Marker: markerPlain}},
		},
		afterMove: mazeObservation{
			At: "b", Marker: markerPlain,
			Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}}, Terminal: true, Revision: 1,
		},
	}
	selector := &mazeRecordingSelector{selected: "C17"}
	result, err := runReferenceMaze(ctx, world, selector, 4)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error=%v, want context.Canceled", err)
	}
	if world.moves != 1 || selector.calls != 0 || len(result.Moves) != 0 || result.Complete {
		t.Fatalf("canceled Move committed response: world moves=%d selector=%d result=%#v", world.moves, selector.calls, result)
	}
}

func TestMazeDiscardsSelectionWhenContextCancelsDuringSelector(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	environment := forkedReferenceMaze(t, "g")
	selector := &cancelingMazeSelector{cancel: cancel}
	result, err := runReferenceMaze(ctx, environment, selector, 12)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error=%v, want context.Canceled", err)
	}
	if selector.calls != 1 || environment.revision != 0 || len(result.Moves) != 0 || len(result.PreferenceFacts) != 0 {
		t.Fatalf("canceled selection changed state: calls=%d revision=%d result=%#v", selector.calls, environment.revision, result)
	}
}

func TestMazeRejectsSelfNeighborAndMissingReverseEdge(t *testing.T) {
	t.Parallel()
	self := &scriptedMazeWorld{observations: []mazeObservation{{
		At: "a", Marker: markerPlain,
		Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}},
	}}}
	if _, err := runReferenceMaze(t.Context(), self, &mazeRecordingSelector{selected: "C17"}, 4); !errors.Is(err, errReferenceMazeInvalid) {
		t.Fatalf("self-neighbor error=%v, want errReferenceMazeInvalid", err)
	}

	missingReverse := &scriptedMazeWorld{observations: []mazeObservation{
		{At: "a", Marker: markerPlain, Neighbors: []publicNeighbor{{Cell: "b", Marker: markerPlain}}},
		{At: "b", Marker: markerPlain, Neighbors: []publicNeighbor{}, Revision: 1},
	}}
	result, err := runReferenceMaze(t.Context(), missingReverse, &mazeRecordingSelector{selected: "C17"}, 4)
	if !errors.Is(err, errReferenceMazeContinuity) || len(result.Moves) != 0 {
		t.Fatalf("missing reverse error=%v result=%#v, want continuity failure before commit", err, result)
	}
}

func TestMazePersistsOnePreferenceAcrossMultipleGenuineForks(t *testing.T) {
	t.Parallel()
	environment := mustReferenceMazeEnvironment(t, referenceMazeSpec{
		Start: "s", Goal: "g",
		Markers: map[cellID]routeMarker{
			"s": markerPlain, "q0": markerQuiet, "v0": markerVivid,
			"x": markerPlain, "q1": markerQuiet, "v1": markerVivid, "g": markerPlain,
		},
		Edges: []referenceMazeEdge{
			{"s", "q0"}, {"s", "v0"}, {"q0", "x"},
			{"x", "q1"}, {"x", "v1"}, {"q1", "g"},
		},
	})
	selector := &mazeRecordingSelector{selected: "C17"}
	result, err := runReferenceMaze(t.Context(), environment, selector, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || selector.calls != 1 || result.SelectorCalls != 1 || len(result.PreferenceFacts) != 1 {
		t.Fatalf("persistent preference result=%#v selector calls=%d, want completion with exactly one", result, selector.calls)
	}
}

type cancelingMazeWorld struct {
	start       mazeObservation
	afterMove   mazeObservation
	cancelStart context.CancelFunc
	cancelMove  context.CancelFunc
	starts      int
	moves       int
}

func (world *cancelingMazeWorld) Start(context.Context) (mazeObservation, error) {
	world.starts++
	if world.cancelStart != nil {
		world.cancelStart()
	}
	return cloneMazeObservation(world.start), nil
}

func (world *cancelingMazeWorld) Move(_ context.Context, _, _ cellID) (mazeObservation, error) {
	world.moves++
	if world.cancelMove != nil {
		world.cancelMove()
	}
	return cloneMazeObservation(world.afterMove), nil
}

type cancelingMazeSelector struct {
	cancel context.CancelFunc
	calls  int
}

func (selector *cancelingMazeSelector) Select(_ context.Context, _ SemanticGap) (CandidateID, error) {
	selector.calls++
	selector.cancel()
	return "C17", nil
}
