package labyrinth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestGenerateScaleFamilyKeepsOneLatentTaskAcrossOneMillionArtifacts(t *testing.T) {
	t.Parallel()
	config := testGeneratorConfig(SuiteCombined, 903_117)
	config.Difficulty = Difficulty{
		WorldSize: 64, RelevantArtifacts: 5, SolutionDepth: 7,
		BranchingFactor: 2, DependencyCount: 4,
	}
	sizes := []int{64, 6_400, 1_000_000}
	cases, family, err := GenerateScaleFamily(config, sizes)
	if err != nil {
		t.Fatal(err)
	}
	if err := family.Validate(); err != nil {
		t.Fatalf("family validation: %v", err)
	}
	if len(cases) != len(sizes) || len(family.Cases) != len(sizes) {
		t.Fatalf("cases=%d descriptor cases=%d", len(cases), len(family.Cases))
	}
	baseOracle := cases[0].PrivateOracle()
	baseScenario := cases[0].ExecutionScenario()
	for index, generated := range cases {
		if err := generated.Validate(); err != nil {
			t.Fatalf("case %d validation: %v", index, err)
		}
		artifact := generated.PublicArtifact()
		if artifact.World.Descriptor.Difficulty.WorldSize != sizes[index] ||
			family.Cases[index].WorldSize != sizes[index] ||
			family.Cases[index].Scenario != artifact.Scenario {
			t.Fatalf("case %d lost exact size/scenario authority", index)
		}
		raw, err := generated.MarshalPublicJSON()
		if err != nil || len(raw) > MaxGeneratedArtifactBytes {
			t.Fatalf("case %d public artifact bytes=%d error=%v", index, len(raw), err)
		}
		if bytes.Contains(raw, []byte(`"seed"`)) || bytes.Contains(raw, []byte("oracle")) {
			t.Fatalf("case %d public artifact exposed hidden generator authority", index)
		}
		oracle := generated.PrivateOracle()
		if !reflect.DeepEqual(oracle.Witness, baseOracle.Witness) ||
			!reflect.DeepEqual(oracle.RequiredEvidence, baseOracle.RequiredEvidence) ||
			!reflect.DeepEqual(generated.ExecutionScenario().Goal(), baseScenario.Goal()) ||
			!reflect.DeepEqual(generated.ExecutionScenario().Catalog(), baseScenario.Catalog()) {
			t.Fatalf("case %d changed the latent task", index)
		}
		transition, _, err := VerifyWitness(generated)
		if err != nil || !transition.Terminal {
			t.Fatalf("case %d witness terminal=%t error=%v", index, transition.Terminal, err)
		}
		assertScaleStartIsBounded(t, generated, index)
	}
	assertScaleCorpusIsVisibleOnlyThroughBoundedActions(t, cases[len(cases)-1])
}

func TestGenerateScaleFamilyRejectsInvalidCoordinatesWithoutPartialResults(t *testing.T) {
	t.Parallel()
	config := testGeneratorConfig(SuiteRetrieve, 8_121)
	for name, sizes := range map[string][]int{
		"missing base": {config.Difficulty.WorldSize + 1, 10_000},
		"unsorted":     {config.Difficulty.WorldSize, 10_000, 1_000},
		"duplicate":    {config.Difficulty.WorldSize, config.Difficulty.WorldSize},
		"too large":    {config.Difficulty.WorldSize, MaxScaleWorldSize + 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cases, descriptor, err := GenerateScaleFamily(config, sizes)
			if !errors.Is(err, ErrInvalidGeneratorConfig) {
				t.Fatalf("error=%v, want invalid generator config", err)
			}
			if cases != nil || descriptor.Schema != "" {
				t.Fatal("invalid scale family returned partial authority")
			}
		})
	}
}

func assertScaleStartIsBounded(t *testing.T, generated GeneratedCase, index int) {
	t.Helper()
	episode, err := cognition.NewEpisodeRef(cognition.EpisodeID("scale-start-" + string(rune('a'+index))))
	if err != nil {
		t.Fatal(err)
	}
	environment, err := NewEnvironment(generated.ExecutionScenario(), episode, func(
		_ context.Context, _ cognition.AttemptRef,
	) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := environment.Start(context.Background(), generated.PublicArtifact().Scenario)
	if err != nil {
		t.Fatal(err)
	}
	if len(started.Observations) != 1 || len(started.Observations[0].Content) > cognition.MaxObservationBytes {
		t.Fatalf("case %d start observation is unbounded", index)
	}
	var payload publicObservationPayload
	if err := json.Unmarshal([]byte(started.Observations[0].Content), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Records) > MaxObservedRecords {
		t.Fatalf("case %d start records=%d", index, len(payload.Records))
	}
}

func assertScaleCorpusIsVisibleOnlyThroughBoundedActions(t *testing.T, generated GeneratedCase) {
	t.Helper()
	episode := cognition.EpisodeRef{ID: "scale-corpus-observation"}
	environment, err := NewEnvironment(generated.ExecutionScenario(), episode, func(
		_ context.Context, actor cognition.AttemptRef,
	) error {
		if actor != witnessActor {
			return cognition.ErrAuthorityDenied
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	started, err := environment.Start(context.Background(), generated.PublicArtifact().Scenario)
	if err != nil {
		t.Fatal(err)
	}
	schema, exists := generated.ExecutionScenario().Catalog().Schema("search")
	if !exists {
		t.Fatal("search schema is absent")
	}
	query := fmt.Sprintf("artifact-%07d", generated.execution.artifactCorpus.ref.Count-1)
	request, err := cognition.NewActionRequest("search", []cognition.ActionArgument{{Name: queryArg, Value: query}})
	if err != nil {
		t.Fatal(err)
	}
	action, err := cognition.NewRegisteredAction("scale-exact-search", witnessActor, schema, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := environment.Apply(context.Background(), episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	var payload publicObservationPayload
	if err := json.Unmarshal([]byte(transition.Observations[0].Content), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Records) != 1 || string(payload.Records[0].ID) != query ||
		payload.Records[0].Content == "" ||
		textSHA256(payload.Records[0].Content) != payload.Records[0].ContentSHA256 ||
		payload.RecordsTruncated {
		t.Fatalf("exact lazy search returned %#v truncated=%t", payload.Records, payload.RecordsTruncated)
	}
	if _, err := NewFilesystemEnvironment(generated.ExecutionScenario(), episode, func(
		_ context.Context, _ cognition.AttemptRef,
	) error {
		return nil
	}); !errors.Is(err, ErrSurfaceLimit) {
		t.Fatalf("filesystem lazy-corpus error=%v, want loud unsupported surface", err)
	}
	if _, err := NewRecordEnvironment(generated.ExecutionScenario(), episode, func(
		_ context.Context, _ cognition.AttemptRef,
	) error {
		return nil
	}); !errors.Is(err, ErrSurfaceLimit) {
		t.Fatalf("record lazy-corpus error=%v, want loud unsupported surface", err)
	}
}
