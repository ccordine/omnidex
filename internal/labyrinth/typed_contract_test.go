package labyrinth

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestGeneratedV1ActionsUseTypedParametersAndNavigationOnlyMovement(t *testing.T) {
	t.Parallel()
	generated, err := Generate(frozenCausalConfigs()[4])
	if err != nil {
		t.Fatal(err)
	}
	expected := map[cognition.ActionKind][]cognition.ActionArgumentName{
		"observe": {}, "search": {queryArg}, "read": {artifactArg},
		"navigate": {fromArg, toArg}, "take": {objectArg},
		"use":   {itemArg, targetArg},
		"write": {expectedSHA256Arg, mutationTargetArg, mutationValueArg, evidenceSetArg},
	}
	for _, action := range generated.execution.definition.actions {
		actual := make([]cognition.ActionArgumentName, len(action.Schema.Parameters))
		for index, parameter := range action.Schema.Parameters {
			actual[index] = parameter.Name
		}
		want := append([]cognition.ActionArgumentName(nil), expected[action.Schema.Kind]...)
		sort.Slice(want, func(left, right int) bool { return want[left] < want[right] })
		if !slices.Equal(actual, want) {
			t.Fatalf("%s parameters=%v want=%v", action.Schema.Kind, actual, want)
		}
		assertMovementParameters(t, action.Schema.Kind, action.Preconditions, action.Effects)
	}
	for _, witness := range generated.oracle.Witness {
		for _, argument := range witness.Request.Arguments {
			if witness.Request.Kind != "navigate" && (argument.Name == fromArg || argument.Name == toArg) {
				t.Fatalf("non-navigation witness %s retained movement argument %s", witness.Request.Kind, argument.Name)
			}
		}
	}
}

func assertMovementParameters(
	t *testing.T,
	kind cognition.ActionKind,
	conditions []Condition,
	effects []Effect,
) {
	t.Helper()
	patterns := make([]PredicatePattern, 0, len(conditions)+len(effects))
	for _, condition := range conditions {
		patterns = append(patterns, condition.Predicate)
	}
	for _, effect := range effects {
		patterns = append(patterns, effect.Predicate)
	}
	for _, pattern := range patterns {
		for _, argument := range pattern.Arguments {
			movement := argument.Parameter == fromArg || argument.Parameter == toArg
			if kind != "navigate" && movement {
				t.Fatalf("non-navigation action %s binds movement through %s", kind, pattern.Name)
			}
		}
		if kind != "navigate" && pattern.Name == "topology.edge" {
			t.Fatalf("non-navigation action %s depends on topology movement", kind)
		}
	}
}

func TestInvalidTypedQueryAndArtifactCannotMutateAnyV1Surface(t *testing.T) {
	t.Parallel()
	generated, err := Generate(frozenCausalConfigs()[4])
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"symbolic", "filesystem", "records"} {
		t.Run(surface, func(t *testing.T) {
			t.Parallel()
			runInvalidTypedActions(t, generated, surface)
		})
	}
}

func TestTypedAcquisitionReturnsEquivalentEvidenceAcrossV1Surfaces(t *testing.T) {
	t.Parallel()
	generated, err := Generate(frozenCausalConfigs()[4])
	if err != nil {
		t.Fatal(err)
	}
	var expected map[EntityID]string
	for _, surface := range []string{"symbolic", "filesystem", "records"} {
		_, acquisition, closeSurface := runAcquisitionThroughSurface(t, generated, surface)
		actual := observedEvidenceRecords(acquisition.Observations[0].Content)
		closeSurface()
		if expected == nil {
			expected = actual
			continue
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("%s evidence=%v want=%v", surface, actual, expected)
		}
	}
	if len(expected) != len(generated.oracle.RequiredEvidence) {
		t.Fatalf("cross-surface evidence count=%d want=%d", len(expected), len(generated.oracle.RequiredEvidence))
	}
}

func runInvalidTypedActions(t *testing.T, generated GeneratedCase, surface string) {
	t.Helper()
	episode := cognition.EpisodeRef{ID: cognition.EpisodeID("invalid-typed-" + surface)}
	authorize := func(_ context.Context, actor cognition.AttemptRef) error {
		if actor != witnessActor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	}
	var environment cognition.Environment
	stateSHA := func() string { return "symbolic" }
	closeSurface := func() {}
	switch surface {
	case "symbolic":
		environment, _ = NewEnvironment(generated.execution, episode, authorize)
	case "filesystem":
		value, err := NewFilesystemEnvironment(generated.execution, episode, authorize)
		if err != nil {
			t.Fatal(err)
		}
		environment, stateSHA, closeSurface = value, value.surfaceStateSHA256, func() { _ = value.Close() }
	case "records":
		value, err := NewRecordEnvironment(generated.execution, episode, authorize)
		if err != nil {
			t.Fatal(err)
		}
		environment, stateSHA, closeSurface = value, func() string {
			return value.kernel.surfaceState.StateSHA256
		}, func() { _ = value.Close() }
	}
	defer closeSurface()
	started, err := environment.Start(context.Background(), generated.public.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	before := stateSHA()
	searchSchema, _ := generated.execution.Catalog().Schema("search")
	invalidQuery := cognition.RegisteredAction{
		ID: "invalid-empty-query", Actor: witnessActor, Schema: searchSchema.Ref(),
		Request:      cognition.ActionRequest{Kind: "search", Arguments: []cognition.ActionArgument{{Name: queryArg}}},
		EvidenceRefs: []cognition.EvidenceRef{},
	}
	if _, err := environment.Apply(
		context.Background(), episode, started.Current, invalidQuery,
	); !errors.Is(err, cognition.ErrInvalidAction) {
		t.Fatalf("empty query error=%v, want invalid action", err)
	}
	readSchema, _ := generated.execution.Catalog().Schema("read")
	unknownRead, err := cognition.NewRegisteredAction(
		"invalid-unknown-artifact", witnessActor, readSchema,
		cognition.ActionRequest{Kind: "read", Arguments: []cognition.ActionArgument{
			{Name: artifactArg, Value: "record-999"},
		}}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.Apply(
		context.Background(), episode, started.Current, unknownRead,
	); !errors.Is(err, cognition.ErrInvalidAction) {
		t.Fatalf("unknown artifact error=%v, want invalid action", err)
	}
	if stateSHA() != before {
		t.Fatal("invalid typed action changed surface authority")
	}
	valid := generated.oracle.Witness[0]
	action, err := cognition.NewRegisteredAction(valid.ID, witnessActor, searchSchema, valid.Request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.Apply(context.Background(), episode, started.Current, action); err != nil {
		t.Fatalf("valid action after two failures: %v", err)
	}
}
