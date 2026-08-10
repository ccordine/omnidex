package labyrinth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestHiddenWorldStateNeverEntersPublicSerialization(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	environment := world.newEnvironment(t)
	started, err := environment.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(world.kernel.definition); !errors.Is(err, ErrPrivateSerialization) {
		t.Fatalf("public definition serialization error = %v, want ErrPrivateSerialization", err)
	}
	privateFirst, err := world.kernel.definition.MarshalPrivateJSON()
	if err != nil {
		t.Fatalf("marshal private definition: %v", err)
	}
	privateSecond, err := world.kernel.definition.MarshalPrivateJSON()
	if err != nil {
		t.Fatalf("marshal private definition again: %v", err)
	}
	if string(privateFirst) != string(privateSecond) || !strings.Contains(string(privateFirst), hiddenStateCanary) {
		t.Fatal("private definition serialization is nondeterministic or omits sealed state")
	}
	privateDigest := sha256.Sum256(privateFirst)
	if hex.EncodeToString(privateDigest[:]) != world.kernel.definition.SHA256() {
		t.Fatal("private definition serialization does not match its content address")
	}
	values := []struct {
		name  string
		value any
	}{
		{"scenario", world.kernel},
		{"environment", environment},
		{"transition", started},
	}
	for _, value := range values {
		value := value
		t.Run(value.name, func(t *testing.T) {
			t.Parallel()
			raw, marshalErr := json.Marshal(value.value)
			if marshalErr != nil {
				t.Fatalf("marshal: %v", marshalErr)
			}
			if strings.Contains(string(raw), hiddenStateCanary) {
				t.Fatalf("hidden state leaked through %s serialization: %s", value.name, raw)
			}
		})
	}
	scenarioJSON, err := json.Marshal(world.kernel)
	if err != nil {
		t.Fatal(err)
	}
	var publicEnvelope struct {
		Scenario cognition.ScenarioRef `json:"scenario"`
		World    json.RawMessage       `json:"world"`
	}
	if err := json.Unmarshal(scenarioJSON, &publicEnvelope); err != nil {
		t.Fatalf("decode public scenario: %v", err)
	}
	publicDigest := sha256.Sum256(publicEnvelope.World)
	if hex.EncodeToString(publicDigest[:]) != publicEnvelope.Scenario.SHA256 {
		t.Fatal("public ScenarioRef does not bind the exact serialized public manifest")
	}

	hiddenTarget := world.action(t, "action-hidden-precondition", "finish", EntityID(hiddenStateCanary))
	_, err = environment.Apply(context.Background(), world.episode, started.Current, hiddenTarget)
	if !errors.Is(err, ErrPrecondition) {
		t.Fatalf("hidden precondition error = %v, want ErrPrecondition", err)
	}
	if strings.Contains(err.Error(), hiddenStateCanary) {
		t.Fatalf("hidden state leaked through failure: %v", err)
	}
}

func TestEquivalentWorldsSerializeIdenticalPublicTransitions(t *testing.T) {
	t.Parallel()
	world := newTestWorld(t)
	left := world.newEnvironment(t)
	right := world.newEnvironment(t)
	leftStart, err := left.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	rightStart, err := right.Start(context.Background(), world.scenario)
	if err != nil {
		t.Fatal(err)
	}
	leftJSON, err := json.Marshal(leftStart)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := json.Marshal(rightStart)
	if err != nil {
		t.Fatal(err)
	}
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("deterministic starts differ:\nleft=%s\nright=%s", leftJSON, rightJSON)
	}
	action := world.action(t, "action-deterministic-1", "enable", "unit-public")
	leftTransition, err := left.Apply(context.Background(), world.episode, leftStart.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	rightTransition, err := right.Apply(context.Background(), world.episode, rightStart.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	leftJSON, _ = json.Marshal(leftTransition)
	rightJSON, _ = json.Marshal(rightTransition)
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("deterministic transitions differ:\nleft=%s\nright=%s", leftJSON, rightJSON)
	}
}

func TestDefinitionRejectsPublicObservationOverflow(t *testing.T) {
	t.Parallel()
	schema := mustActionSchema(t, "symbolic.noop.v1", "noop", nil)
	catalog, err := cognition.NewActionCatalog("symbolic.overflow.v1", "1.0.0", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	entities := make([]Entity, MaxPublicPredicates+1)
	facts := make([]cognition.Predicate, MaxPublicPredicates+1)
	for index := range entities {
		id := EntityID(fmt.Sprintf("unit-%03d", index))
		entities[index] = Entity{ID: id, Kind: "unit", Public: true}
		facts[index] = mustPredicate(t, "state.visible", string(id))
	}
	goal := mustGoal(t, mustPredicate(t, "state.done", string(entities[0].ID)))
	_, err = NewDefinition(
		catalog,
		entities,
		[]PredicateSchema{
			{Name: "state.visible", ArgumentKinds: []EntityKind{"unit"}, Public: true},
			{Name: "state.done", ArgumentKinds: []EntityKind{"unit"}},
		},
		facts,
		[]ActionDefinition{{Schema: schema, Cost: 1}},
		goal,
	)
	if !errors.Is(err, ErrObservationLimit) {
		t.Fatalf("public observation overflow error = %v, want ErrObservationLimit", err)
	}
}

func TestDefinitionRejectsPublicObservationByteOverflow(t *testing.T) {
	t.Parallel()
	actionSchema := mustActionSchema(t, "symbolic.byte-noop.v1", "byte-noop", nil)
	catalog, err := cognition.NewActionCatalog(
		"symbolic.byte-overflow.v1", "1.0.0", []cognition.ActionSchema{actionSchema},
	)
	if err != nil {
		t.Fatal(err)
	}
	const argumentCount = cognition.MaxPredicateArgs
	entities := make([]Entity, 0, argumentCount+40)
	shared := make([]string, argumentCount)
	for index := range shared {
		id := fmt.Sprintf("shared-%02d-%s", index, strings.Repeat("x", 108))
		shared[index] = id
		entities = append(entities, Entity{ID: EntityID(id), Kind: "unit", Public: true})
	}
	facts := make([]cognition.Predicate, 40)
	for index := range facts {
		id := fmt.Sprintf("unique-%02d-%s", index, strings.Repeat("x", 108))
		entities = append(entities, Entity{ID: EntityID(id), Kind: "unit", Public: true})
		arguments := append([]string(nil), shared...)
		arguments[0] = id
		facts[index] = mustPredicate(t, "state.verbose", arguments...)
	}
	kinds := make([]EntityKind, argumentCount)
	for index := range kinds {
		kinds[index] = "unit"
	}
	goal := mustGoal(t, mustPredicate(t, "state.done", shared[0]))
	_, err = NewDefinition(
		catalog,
		entities,
		[]PredicateSchema{
			{Name: "state.verbose", ArgumentKinds: kinds, Public: true},
			{Name: "state.done", ArgumentKinds: []EntityKind{"unit"}},
		},
		facts,
		[]ActionDefinition{{Schema: actionSchema, Cost: 1}},
		goal,
	)
	if !errors.Is(err, ErrObservationLimit) {
		t.Fatalf("public observation byte overflow error = %v, want ErrObservationLimit", err)
	}
}

func mustGoal(t *testing.T, predicate cognition.Predicate) cognition.GoalExpression {
	t.Helper()
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	return goal
}
