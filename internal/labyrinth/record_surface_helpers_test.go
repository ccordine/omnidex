package labyrinth

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func generatedForRecordSurfaceTest(t *testing.T, seed uint64) GeneratedCase {
	t.Helper()
	config := testGeneratorConfig(SuiteCombined, seed)
	config.Difficulty.SolutionDepth = 7
	config.Difficulty.DependencyCount = 5
	generated, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	return generated
}

func recordSurfaceTestActor() cognition.AttemptRef {
	return cognition.AttemptRef{
		JobID: 92, Generation: 2, StepID: 8, Attempt: 1, WorkerID: "record-worker",
	}
}

func startRecordSurfaceTest(
	t *testing.T,
	generated GeneratedCase,
	episodeID cognition.EpisodeID,
) (*RecordEnvironment, cognition.AttemptRef, cognition.EpisodeRef, cognition.Transition) {
	t.Helper()
	actor := recordSurfaceTestActor()
	episode := cognition.EpisodeRef{ID: episodeID}
	environment, err := NewRecordEnvironment(
		generated.ExecutionScenario(), episode,
		func(_ context.Context, candidate cognition.AttemptRef) error {
			if candidate != actor {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := environment.Start(context.Background(), generated.public.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	return environment, actor, episode, transition
}

func registeredRecordAction(
	t *testing.T,
	scenario Scenario,
	actor cognition.AttemptRef,
	witness WitnessAction,
	evidence ...cognition.EvidenceRef,
) cognition.RegisteredAction {
	t.Helper()
	schema, exists := scenario.Catalog().Schema(witness.Request.Kind)
	if !exists {
		t.Fatalf("schema %s absent", witness.Request.Kind)
	}
	if schema.EvidencePolicy != cognition.EvidenceRequired {
		evidence = nil
	}
	action, err := cognition.NewRegisteredAction(
		witness.ID, actor, schema, witness.Request, evidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func observationEvidenceRefs(observations []cognition.Observation) []cognition.EvidenceRef {
	refs := make([]cognition.EvidenceRef, len(observations))
	for index, observation := range observations {
		refs[index] = observation.EvidenceRef()
	}
	return refs
}
