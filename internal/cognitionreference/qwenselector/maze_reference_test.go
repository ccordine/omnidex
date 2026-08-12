package qwenselector_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMazeForcedCorridorCompletesWithoutSelector(t *testing.T) {
	t.Parallel()
	environment := mustReferenceMazeEnvironment(t, referenceMazeSpec{
		Start: "c0", Goal: "c4",
		Markers: map[cellID]routeMarker{"c0": markerPlain, "c1": markerQuiet, "c2": markerPlain, "c3": markerVivid, "c4": markerPlain},
		Edges:   []referenceMazeEdge{{"c0", "c1"}, {"c1", "c2"}, {"c2", "c3"}, {"c3", "c4"}},
	})
	selector := &mazeRecordingSelector{selected: "C17"}
	result, err := runReferenceMaze(t.Context(), environment, selector, 16)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.SelectorCalls != 0 || selector.calls != 0 {
		t.Fatalf("complete=%v selector calls=%d/%d, want true and 0/0", result.Complete, result.SelectorCalls, selector.calls)
	}
	want := []mazeMove{{"c0", "c1", mazeMoveForward}, {"c1", "c2", mazeMoveForward}, {"c2", "c3", mazeMoveForward}, {"c3", "c4", mazeMoveForward}}
	if !reflect.DeepEqual(result.Moves, want) {
		t.Fatalf("moves=%#v, want %#v", result.Moves, want)
	}
}

func TestProceduralTinyMazesCompleteWithPinnedSelectorCounts(t *testing.T) {
	t.Parallel()
	for _, seed := range []uint64{11, 23, 41, 67, 89, 131} {
		seed := seed
		for _, candidate := range []CandidateID{"C17", "C23"} {
			candidate := candidate
			t.Run(testSeedName(seed, candidate), func(t *testing.T) {
				t.Parallel()
				environment, err := generateReferenceMaze(seed)
				if err != nil {
					t.Fatal(err)
				}
				selector := &mazeRecordingSelector{selected: candidate}
				result, err := runReferenceMaze(t.Context(), environment, selector, 20)
				if err != nil {
					t.Fatal(err)
				}
				const pinnedSelectorCalls = 1
				if !result.Complete || result.SelectorCalls != pinnedSelectorCalls || selector.calls != pinnedSelectorCalls {
					t.Fatalf("complete=%v selector calls=%d/%d, want true and %d", result.Complete, result.SelectorCalls, selector.calls, pinnedSelectorCalls)
				}
				if len(result.PreferenceFacts) != 1 || result.PreferenceFacts[0].CandidateID != candidate {
					t.Fatalf("preference facts=%#v, want one persistent fact for %q", result.PreferenceFacts, candidate)
				}
				assertMazeMovesLegal(t, environment.spec, result.Moves)
			})
		}
	}
}

func TestMazeBacktrackingAndForcedMovesDoNotRecallSelector(t *testing.T) {
	t.Parallel()
	environment := mustReferenceMazeEnvironment(t, referenceMazeSpec{
		Start: "s", Goal: "g",
		Markers: map[cellID]routeMarker{"s": markerPlain, "a": markerQuiet, "a1": markerPlain, "b": markerVivid, "g": markerPlain},
		Edges:   []referenceMazeEdge{{"s", "a"}, {"a", "a1"}, {"s", "b"}, {"b", "g"}},
	})
	selector := &mazeRecordingSelector{selected: "C17"}
	result, err := runReferenceMaze(t.Context(), environment, selector, 16)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || selector.calls != 1 || result.SelectorCalls != 1 {
		t.Fatalf("complete=%v selector calls=%d/%d, want true and 1/1", result.Complete, selector.calls, result.SelectorCalls)
	}
	want := []mazeMove{{"s", "a", mazeMoveForward}, {"a", "a1", mazeMoveForward}, {"a1", "a", mazeMoveBacktrack}, {"a", "s", mazeMoveBacktrack}, {"s", "b", mazeMoveForward}, {"b", "g", mazeMoveForward}}
	if !reflect.DeepEqual(result.Moves, want) {
		t.Fatalf("moves=%#v, want %#v", result.Moves, want)
	}
	if _, exists := result.DeadEnds["a1"]; !exists {
		t.Fatalf("dead-end bookkeeping=%#v, want a1", result.DeadEnds)
	}
}

func TestMazeSameMarkerForkUsesCodeOwnedTraversalWithoutSelector(t *testing.T) {
	t.Parallel()
	environment := mustReferenceMazeEnvironment(t, referenceMazeSpec{
		Start: "s", Goal: "g",
		Markers: map[cellID]routeMarker{
			"s": markerPlain, "a": markerQuiet, "b": markerQuiet, "g": markerPlain,
		},
		Edges: []referenceMazeEdge{{"s", "a"}, {"s", "b"}, {"b", "g"}},
	})
	result, err := runReferenceMaze(t.Context(), environment, nil, 12)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.SelectorCalls != 0 || len(result.PreferenceFacts) != 0 {
		t.Fatalf("code-owned traversal result=%#v, want completion with zero inference", result)
	}
}

func TestMazeInvalidSemanticSelectionCannotMoveOrFallback(t *testing.T) {
	t.Parallel()
	for name, selector := range map[string]*mazeRecordingSelector{
		"unknown": {selected: "C99"}, "failure": {err: errors.New("provider failed")},
	} {
		name, selector := name, selector
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			environment := forkedReferenceMaze(t, "g")
			result, err := runReferenceMaze(t.Context(), environment, selector, 12)
			if err == nil {
				t.Fatal("Run() error=nil, want explicit semantic failure")
			}
			if len(result.Moves) != 0 || environment.revision != 0 || environment.current != environment.spec.Start {
				t.Fatalf("invalid selection changed world: result=%#v revision=%d current=%q", result, environment.revision, environment.current)
			}
		})
	}
}

func TestMazeFailsLoudlyAtBoundOrDisconnectedWorld(t *testing.T) {
	t.Parallel()
	disconnected := mustReferenceMazeEnvironment(t, referenceMazeSpec{
		Start: "s", Goal: "g", Markers: map[cellID]routeMarker{"s": markerPlain, "g": markerQuiet},
	})
	if _, err := runReferenceMaze(t.Context(), disconnected, &mazeRecordingSelector{selected: "C17"}, 4); !errors.Is(err, errReferenceMazeUnresolvable) {
		t.Fatalf("disconnected error=%v, want errReferenceMazeUnresolvable", err)
	}
	corridor := mustReferenceMazeEnvironment(t, referenceMazeSpec{
		Start: "a", Goal: "c", Markers: map[cellID]routeMarker{"a": markerPlain, "b": markerPlain, "c": markerPlain},
		Edges: []referenceMazeEdge{{"a", "b"}, {"b", "c"}},
	})
	if _, err := runReferenceMaze(t.Context(), corridor, &mazeRecordingSelector{selected: "C17"}, 1); !errors.Is(err, errReferenceMazeBound) {
		t.Fatalf("bounded error=%v, want errReferenceMazeBound", err)
	}
}

type mazeRecordingSelector struct {
	selected CandidateID
	err      error
	calls    int
	gaps     []SemanticGap
}

func (selector *mazeRecordingSelector) Select(_ context.Context, gap SemanticGap) (CandidateID, error) {
	selector.calls++
	selector.gaps = append(selector.gaps, gap.Clone())
	return selector.selected, selector.err
}
