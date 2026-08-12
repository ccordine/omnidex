package qwenselector_test

import (
	"fmt"
	"testing"
)

func mustReferenceMazeEnvironment(t *testing.T, spec referenceMazeSpec) *referenceMazeEnvironment {
	t.Helper()
	environment, err := newReferenceMazeEnvironment(spec)
	if err != nil {
		t.Fatal(err)
	}
	return environment
}

func forkedReferenceMaze(t *testing.T, goal cellID) *referenceMazeEnvironment {
	t.Helper()
	return mustReferenceMazeEnvironment(t, referenceMazeSpec{
		Start: "s", Goal: goal,
		Markers: map[cellID]routeMarker{
			"s": markerPlain, "q": markerQuiet, "v": markerVivid, "g": markerPlain, "d": markerPlain,
		},
		Edges: []referenceMazeEdge{{"s", "q"}, {"s", "v"}, {"q", "g"}, {"v", "d"}},
	})
}

func assertMazeMovesLegal(t *testing.T, spec referenceMazeSpec, moves []mazeMove) {
	t.Helper()
	legal := make(map[string]struct{}, 2*len(spec.Edges))
	for _, edge := range spec.Edges {
		legal[string(edge.Left)+"\x00"+string(edge.Right)] = struct{}{}
		legal[string(edge.Right)+"\x00"+string(edge.Left)] = struct{}{}
	}
	for _, move := range moves {
		if _, exists := legal[string(move.From)+"\x00"+string(move.To)]; !exists {
			t.Fatalf("movement %#v is outside authoritative topology", move)
		}
	}
}

func testSeedName(seed uint64, candidate CandidateID) string {
	return fmt.Sprintf("seed-%d/%s", seed, candidate)
}
