package qwenselector_test

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestMazeSemanticPacketIsIdenticalAcrossPrivateContinuations(t *testing.T) {
	t.Parallel()
	first := forkedReferenceMaze(t, "g")
	second := forkedReferenceMaze(t, "d")
	firstSelector := &mazeRecordingSelector{selected: "C17", err: errStopAfterGap}
	secondSelector := &mazeRecordingSelector{selected: "C17", err: errStopAfterGap}
	_, firstErr := runReferenceMaze(t.Context(), first, firstSelector, 12)
	_, secondErr := runReferenceMaze(t.Context(), second, secondSelector, 12)
	if firstErr == nil || secondErr == nil {
		t.Fatal("probe selectors unexpectedly permitted movement")
	}
	if first.revision != 0 || second.revision != 0 || len(firstSelector.gaps) != 1 || len(secondSelector.gaps) != 1 {
		t.Fatalf("gap probes changed reality or did not see exactly one packet")
	}
	if !reflect.DeepEqual(firstSelector.gaps[0], secondSelector.gaps[0]) {
		t.Fatalf("same public surface leaked private continuation: first=%#v second=%#v", firstSelector.gaps[0], secondSelector.gaps[0])
	}
}

func TestMazeSemanticPacketContainsOnlyPublicMeaningAndOpaqueCandidates(t *testing.T) {
	t.Parallel()
	environment := forkedReferenceMaze(t, "g")
	selector := &mazeRecordingSelector{selected: "C17"}
	result, err := runReferenceMaze(t.Context(), environment, selector, 12)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(selector.gaps) != 1 {
		t.Fatalf("complete=%v gap count=%d, want true and one", result.Complete, len(selector.gaps))
	}
	gap := selector.gaps[0]
	if !reflect.DeepEqual(gap, referenceMazeGap()) {
		t.Fatalf("selector packet=%#v, want exact public semantic remainder", gap)
	}
	rendered := strings.ToLower(gap.Question)
	for _, evidence := range gap.Evidence {
		rendered += " " + strings.ToLower(evidence.Content)
	}
	for _, candidate := range gap.Candidates {
		rendered += " " + strings.ToLower(candidate.Summary)
	}
	for _, forbidden := range []string{"cell", "direction", "move", "operation", "seed", "solution", "distance", "score", "goal"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("semantic packet leaks forbidden mechanics/private token %q: %s", forbidden, rendered)
		}
	}
	privateIDs := map[string]struct{}{"q": {}, "v": {}, "g": {}, "d": {}}
	for _, token := range strings.FieldsFunc(rendered, func(character rune) bool {
		return character < 'a' || character > 'z'
	}) {
		if _, forbidden := privateIDs[token]; forbidden {
			t.Fatalf("semantic packet leaks private cell identity %q: %s", token, rendered)
		}
	}
}

func TestMazeCompletionComesOnlyFromAuthoritativeObservation(t *testing.T) {
	t.Parallel()
	world := &scriptedMazeWorld{observations: []mazeObservation{
		{At: "a", Marker: markerPlain, Neighbors: []publicNeighbor{{Cell: "b", Marker: markerPlain}}},
		{At: "b", Marker: markerPlain, Neighbors: []publicNeighbor{{Cell: "a", Marker: markerPlain}}, Terminal: true, Revision: 1},
	}}
	result, err := runReferenceMaze(t.Context(), world, &mazeRecordingSelector{selected: "C17"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || world.moves != 1 {
		t.Fatalf("complete=%v moves=%d, want authoritative completion after one move", result.Complete, world.moves)
	}
}

func TestMazeRejectsEnvironmentResponsesOutsideExactTransitionContinuity(t *testing.T) {
	t.Parallel()
	validStart := mazeObservation{
		At: "a", Marker: markerPlain,
		Neighbors: []publicNeighbor{{Cell: "b", Marker: markerPlain}}, Revision: 0,
	}
	for _, test := range []struct {
		name         string
		observations []mazeObservation
		wantMoves    int
	}{
		{
			name: "nonzero initial revision",
			observations: []mazeObservation{{
				At: "a", Marker: markerPlain,
				Neighbors: []publicNeighbor{{Cell: "b", Marker: markerPlain}}, Revision: 1,
			}},
		},
		{
			name: "wrong destination",
			observations: []mazeObservation{validStart, {
				At: "c", Marker: markerPlain, Neighbors: []publicNeighbor{}, Revision: 1,
			}},
			wantMoves: 1,
		},
		{
			name: "unchanged revision",
			observations: []mazeObservation{validStart, {
				At: "b", Marker: markerPlain, Neighbors: []publicNeighbor{}, Revision: 0,
			}},
			wantMoves: 1,
		},
		{
			name: "skipped revision",
			observations: []mazeObservation{validStart, {
				At: "b", Marker: markerPlain, Neighbors: []publicNeighbor{}, Revision: 2,
			}},
			wantMoves: 1,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			world := &scriptedMazeWorld{observations: test.observations}
			result, err := runReferenceMaze(t.Context(), world, &mazeRecordingSelector{selected: "C17"}, 4)
			if !errors.Is(err, errReferenceMazeContinuity) {
				t.Fatalf("Run() error=%v, want errReferenceMazeContinuity", err)
			}
			if world.moves != test.wantMoves || len(result.Moves) != 0 || result.Complete {
				t.Fatalf("world moves=%d result=%#v, want calls=%d and no accepted transition", world.moves, result, test.wantMoves)
			}
		})
	}
}

func TestMazeRunnerSourceCannotSeePrivateAuthorityOrExposeModelActions(t *testing.T) {
	t.Parallel()
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "maze_runner_test.go", nil, parser.AllErrors)
	if err != nil {
		t.Fatal(err)
	}
	forbiddenIdentifiers := map[string]struct{}{
		"Goal": {}, "Seed": {}, "Solution": {}, "Distance": {}, "Score": {},
		"OperationID": {}, "Argument": {}, "Action": {}, "Tool": {}, "Direction": {},
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok {
			if _, forbidden := forbiddenIdentifiers[identifier.Name]; forbidden {
				t.Errorf("maze runner contains forbidden authority/model-action identifier %q", identifier.Name)
			}
		}
		return true
	})
	raw, err := os.ReadFile("maze_runner_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"json:\"move", "json:\"action", "json:\"tool", "CandidateIDTo"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("maze runner contains forbidden model surface %q", forbidden)
		}
	}
}

var errStopAfterGap = &gapProbeError{}

type gapProbeError struct{}

func (*gapProbeError) Error() string { return "stop after public semantic gap" }

type scriptedMazeWorld struct {
	observations []mazeObservation
	moves        int
}

func (world *scriptedMazeWorld) Start(context.Context) (mazeObservation, error) {
	return cloneMazeObservation(world.observations[0]), nil
}

func (world *scriptedMazeWorld) Move(_ context.Context, _, _ cellID) (mazeObservation, error) {
	world.moves++
	return cloneMazeObservation(world.observations[world.moves]), nil
}
