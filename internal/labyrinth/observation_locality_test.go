package labyrinth

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestModelObservationProjectsOnlyLocalTopologyFromALargerWorld(t *testing.T) {
	config := frozenCausalConfigs()[4]
	generated, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated.public.World.Entities) <= 25 {
		t.Fatalf("fixture has only %d public entities", len(generated.public.World.Entities))
	}
	distant := cognition.Predicate{Name: "topology.edge", Args: []string{"stage-005", "stage-006"}}
	if !predicateIn(generated.public.World.InitialFacts, distant) {
		t.Fatal("public environment manifest lacks the distant topology control edge")
	}

	for _, surface := range []string{"symbolic", "filesystem", "records"} {
		t.Run(surface, func(t *testing.T) {
			start, acquisition, closeSurface := runAcquisitionThroughSurface(t, generated, surface)
			defer closeSurface()
			started := observedPredicates(t, start, surface)
			assertOnlyOutgoingFrom(t, started, "stage-000", distant)
			afterSearch := observedPredicates(t, acquisition, surface)
			assertOnlyOutgoingFrom(t, afterSearch, "stage-000", distant)
		})
	}
}

func observedPredicates(
	t *testing.T,
	transition cognition.Transition,
	surface string,
) []cognition.Predicate {
	t.Helper()
	raw := json.RawMessage(transition.Observations[0].Content)
	if surface != "symbolic" {
		var envelope struct {
			SymbolicState json.RawMessage `json:"symbolic_state"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		raw = envelope.SymbolicState
	}
	var payload struct {
		Predicates []cognition.Predicate `json:"predicates"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Predicates
}

func assertOnlyOutgoingFrom(
	t *testing.T,
	predicates []cognition.Predicate,
	current string,
	distant cognition.Predicate,
) {
	t.Helper()
	edges := 0
	for _, predicate := range predicates {
		if predicateKey(predicate) == predicateKey(distant) {
			t.Fatalf("distant edge leaked while current=%s", current)
		}
		if predicate.Name != "topology.edge" {
			continue
		}
		edges++
		if len(predicate.Args) != 2 || predicate.Args[0] != current {
			t.Fatalf("non-local edge visible at %s: %#v", current, predicate)
		}
	}
	if edges == 0 || edges > MaxBranchingFactor+1 {
		t.Fatalf("local exits=%d outside [1,%d]", edges, MaxBranchingFactor+1)
	}
}

func predicateIn(values []cognition.Predicate, expected cognition.Predicate) bool {
	for _, predicate := range values {
		if predicateKey(predicate) == predicateKey(expected) {
			return true
		}
	}
	return false
}

func TestTopologyWithoutACurrentMarkerIsNotModelVisible(t *testing.T) {
	facts := newFactSet([]cognition.Predicate{{Name: "topology.edge", Args: []string{"left", "right"}}})
	current, exists := observationCurrentLocation(facts)
	if topologyVisibleAt(facts.sorted()[0], current, exists) {
		t.Fatal("topology without a legal current location became model-visible")
	}
}

func TestSymbolicRecordProjectionReportsItsHardBound(t *testing.T) {
	records := make([]PublicRecord, MaxObservedRecords+1)
	for index := range records {
		records[index] = PublicRecord{ID: EntityID("record"), Location: "stage-000"}
	}
	facts := newFactSet([]cognition.Predicate{{Name: "surface.marker", Args: []string{"stage-000"}}})
	visible, truncated := observedRecords(records, nil, facts, nil)
	if len(visible) != MaxObservedRecords || !truncated {
		t.Fatalf("visible=%d truncated=%t", len(visible), truncated)
	}
}
