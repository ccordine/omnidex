package qwenselector_test

import (
	"errors"
	"testing"
)

func TestMazeRejectsInvalidMoveObservationBeforeAcceptingTrace(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		returned mazeObservation
	}{
		{
			name: "self neighbor",
			returned: mazeObservation{
				At: "b", Marker: markerPlain,
				Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}, {Cell: "b", Marker: markerPlain}}, Revision: 1,
			},
		},
		{
			name: "duplicate neighbor",
			returned: mazeObservation{
				At: "b", Marker: markerPlain,
				Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}, {Cell: "a", Marker: markerPlain}}, Revision: 1,
			},
		},
		{
			name: "invalid extra neighbor marker",
			returned: mazeObservation{
				At: "b", Marker: markerPlain,
				Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}, {Cell: "c", Marker: "hidden"}}, Revision: 1,
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			world := &scriptedMazeWorld{observations: []mazeObservation{staticMazeStart(), test.returned}}
			result, err := runReferenceMaze(t.Context(), world, nil, 2)
			if !errors.Is(err, errReferenceMazeInvalid) {
				t.Fatalf("Run() error=%v, want errReferenceMazeInvalid", err)
			}
			if world.moves != 1 || len(result.Moves) != 0 || result.Complete {
				t.Fatalf("invalid returned observation was accepted: world moves=%d result=%#v", world.moves, result)
			}
		})
	}
}

func TestMazeRejectsReverseEdgeWithWrongPublicMarker(t *testing.T) {
	t.Parallel()
	world := &scriptedMazeWorld{observations: []mazeObservation{
		staticMazeStart(),
		{
			At: "b", Marker: markerPlain,
			Neighbors: []publicNeighbor{{Cell: "a", Marker: markerVivid}}, Revision: 1,
		},
	}}
	result, err := runReferenceMaze(t.Context(), world, nil, 2)
	if !errors.Is(err, errReferenceMazeContinuity) || len(result.Moves) != 0 {
		t.Fatalf("reverse marker error=%v result=%#v, want continuity failure before trace acceptance", err, result)
	}
}

func TestMazeRejectsEveryStaticPublicSurfaceChangeOnRevisit(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*mazeObservation)
	}{
		{"marker", func(observation *mazeObservation) { observation.Marker = markerVivid }},
		{"terminal", func(observation *mazeObservation) { observation.Terminal = true }},
		{"added neighbor", func(observation *mazeObservation) {
			observation.Neighbors = append(observation.Neighbors, publicNeighbor{Cell: "d", Marker: markerPlain})
		}},
		{"removed neighbor", func(observation *mazeObservation) { observation.Neighbors = observation.Neighbors[:1] }},
		{"changed neighbor marker", func(observation *mazeObservation) { observation.Neighbors[1].Marker = markerVivid }},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			revisited := staticMazeRevisit()
			test.mutate(&revisited)
			world := &scriptedMazeWorld{observations: []mazeObservation{
				staticMazeForkStart(), staticMazeBranch(), revisited,
			}}
			result, err := runReferenceMaze(t.Context(), world, nil, 4)
			if !errors.Is(err, errReferenceMazeContinuity) {
				t.Fatalf("Run() error=%v, want errReferenceMazeContinuity", err)
			}
			if world.moves != 2 || len(result.Moves) != 1 || result.Complete {
				t.Fatalf("changed revisit was accepted: world moves=%d result=%#v", world.moves, result)
			}
		})
	}
}

func TestMazeAcceptsCanonicalReorderedSurfaceOnRevisit(t *testing.T) {
	t.Parallel()
	revisited := staticMazeRevisit()
	revisited.Neighbors[0], revisited.Neighbors[1] = revisited.Neighbors[1], revisited.Neighbors[0]
	world := &scriptedMazeWorld{observations: []mazeObservation{
		staticMazeForkStart(), staticMazeBranch(), revisited,
		{
			At: "c", Marker: markerQuiet,
			Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}}, Terminal: true, Revision: 3,
		},
	}}
	result, err := runReferenceMaze(t.Context(), world, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Moves) != 3 {
		t.Fatalf("canonical reordered surface result=%#v, want exact-bound completion", result)
	}
}

func TestMazeCompletesWhenTerminalArrivesOnExactMoveBound(t *testing.T) {
	t.Parallel()
	world := &scriptedMazeWorld{observations: []mazeObservation{
		staticMazeStart(),
		{
			At: "b", Marker: markerPlain,
			Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}}, Terminal: true, Revision: 1,
		},
	}}
	result, err := runReferenceMaze(t.Context(), world, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Moves) != 1 {
		t.Fatalf("exact-bound result=%#v, want completion after one accepted move", result)
	}
}

func staticMazeStart() mazeObservation {
	return mazeObservation{
		At: "a", Marker: markerPlain,
		Neighbors: []publicNeighbor{{Cell: "b", Marker: markerPlain}},
	}
}

func staticMazeForkStart() mazeObservation {
	return mazeObservation{
		At: "a", Marker: markerPlain,
		Neighbors: []publicNeighbor{{Cell: "b", Marker: markerQuiet}, {Cell: "c", Marker: markerQuiet}},
	}
}

func staticMazeBranch() mazeObservation {
	return mazeObservation{
		At: "b", Marker: markerQuiet,
		Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}}, Revision: 1,
	}
}

func staticMazeRevisit() mazeObservation {
	return mazeObservation{
		At: "a", Marker: markerPlain,
		Neighbors: []publicNeighbor{{Cell: "b", Marker: markerQuiet}, {Cell: "c", Marker: markerQuiet}}, Revision: 2,
	}
}
