package qwenselector_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/gryph/omnidex/internal/cognitionreference/qwenselector"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

var liveMazeForbiddenMechanicTokens = []string{
	"goal", "goals", "seed", "seeds", "terminal", "terminals",
	"revision", "revisions", "topology", "topologies", "edge", "edges",
	"neighbor", "neighbors", "coordinate", "coordinates", "row", "rows",
	"column", "columns", "north", "south", "east", "west", "northeast",
	"northwest", "southeast", "southwest", "direction", "directions",
	"operation", "operations", "action", "actions", "tool", "tools",
	"move", "moves", "argument", "arguments",
}

func TestLiveContaminatedQwenTinyMazeVertical(t *testing.T) {
	if os.Getenv("OMNIDEX_COGNITION_MAZE_SMOKE") != "1" {
		t.Skip("set OMNIDEX_COGNITION_MAZE_SMOKE=1 to run the real-Qwen tiny-maze smoke")
	}
	t.Log("TINY MAZE COGNITION SMOKE — CONTAMINATED — NON-PROMOTABLE — IN-MEMORY DEVELOPMENT EVIDENCE ONLY")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	endpoint := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	if endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	recorder := &liveExactRecorder{Client: ollama.New(
		endpoint, liveQwenModel, "", 15*time.Minute, liveQwenContext,
	)}
	selection := llm.ProviderIdentitySelection{
		Model: liveQwenModel, NativeContextLimit: liveQwenContext,
	}
	provider, err := llm.RequireDiscoveredProviderIdentityEvidence(
		ctx, recorder, selection, "contaminated-cognition-maze-smoke.v1",
	)
	if err != nil {
		t.Fatalf("discover exact installed Qwen identity: %v", err)
	}
	selector, err := qwenselector.New(
		recorder, provider, qwenselector.Limits{MaxInputTokens: 4096, MaxOutputTokens: 64},
	)
	if err != nil {
		t.Fatal(err)
	}

	const seed uint64 = 913
	spec := liveReferenceMazeSpec(seed)
	environment := mustReferenceMazeEnvironment(t, spec)
	result, err := runReferenceMaze(ctx, environment, selector, 20)
	if err != nil {
		logLiveExactEvidence(t, recorder)
		t.Fatal(err)
	}
	if recorder.calls != 1 || recorder.dispatched != 1 || result.SelectorCalls != 1 ||
		len(result.PreferenceFacts) != 1 {
		t.Fatalf(
			"provider/selector/fact counts=%d/%d/%d/%d, want exactly 1/1/1/1",
			recorder.calls, recorder.dispatched, result.SelectorCalls, len(result.PreferenceFacts),
		)
	}
	if !result.Complete || !containsMazeMoveKind(result.Moves, mazeMoveBacktrack) {
		t.Fatalf("live maze result=%#v, want authoritative completion with code-owned backtracking", result)
	}
	assertLivePreferenceFact(t, result.PreferenceFacts[0])
	assertMazeMovesLegal(t, environment.spec, result.Moves)
	assertLiveMazeModelSurface(t, recorder, spec)
	logLiveExactEvidence(t, recorder)
}

func liveReferenceMazeSpec(seed uint64) referenceMazeSpec {
	start := cellID("private-start-913")
	markers := map[cellID]routeMarker{start: markerPlain}
	edges := make([]referenceMazeEdge, 0, 8)
	addReferenceMazeArm(&edges, markers, start, "private-a-quiet-", markerQuiet, 1+int(seed%2))
	addReferenceMazeArm(&edges, markers, start, "private-b-vivid-", markerVivid, 1+int((seed/2)%2))
	goal := addReferenceMazeArm(&edges, markers, start, "private-z-goal-", markerPlain, 1+int((seed/4)%2))
	return referenceMazeSpec{Start: start, Goal: goal, Markers: markers, Edges: edges}
}

func containsMazeMoveKind(moves []mazeMove, wanted mazeMoveKind) bool {
	for _, move := range moves {
		if move.Kind == wanted {
			return true
		}
	}
	return false
}

func assertLivePreferenceFact(t *testing.T, fact routePreferenceFact) {
	t.Helper()
	if fact.GapID != referenceMazeGap().ID {
		t.Fatalf("preference fact gap=%q, want exact code-held gap", fact.GapID)
	}
	switch fact.CandidateID {
	case "C17":
		if fact.Marker != markerQuiet {
			t.Fatalf("C17 materialized marker=%q, want quiet", fact.Marker)
		}
	case "C23":
		if fact.Marker != markerVivid {
			t.Fatalf("C23 materialized marker=%q, want vivid", fact.Marker)
		}
	default:
		t.Fatalf("materialized candidate=%q, want exact gap member", fact.CandidateID)
	}
}

func assertLiveMazeModelSurface(
	t *testing.T,
	recorder *liveExactRecorder,
	spec referenceMazeSpec,
) {
	t.Helper()
	schema, err := json.Marshal(recorder.prepared.ResponseSchema)
	if err != nil {
		t.Fatal(err)
	}
	visible := strings.ToLower(recorder.prepared.Prompt + "\n" + string(schema))
	for _, required := range []string{"c17", "c23", "quiet", "vivid", "candidate_id"} {
		if !strings.Contains(visible, required) {
			t.Fatalf("model surface omitted required public semantic token %q: %s", required, visible)
		}
	}
	for cell := range spec.Markers {
		if strings.Contains(visible, strings.ToLower(string(cell))) {
			t.Fatalf("model surface leaked private cell identity %q: %s", cell, visible)
		}
	}
	for _, forbidden := range []string{"gap.route-style", "objective.reference-navigation"} {
		if strings.Contains(visible, forbidden) {
			t.Fatalf("model surface leaked private authority/mechanics token %q: %s", forbidden, visible)
		}
	}
	if forbidden, found := findForbiddenMazeMechanic(visible); found {
		t.Fatalf("model surface leaked private authority/mechanics token %q: %s", forbidden, visible)
	}
}

func findForbiddenMazeMechanic(value string) (string, bool) {
	forbidden := make(map[string]struct{}, len(liveMazeForbiddenMechanicTokens))
	for _, token := range liveMazeForbiddenMechanicTokens {
		forbidden[token] = struct{}{}
	}
	words := strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character)
	})
	for _, word := range words {
		if _, exists := forbidden[word]; exists {
			return word, true
		}
	}
	return "", false
}
