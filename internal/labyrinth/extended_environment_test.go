package labyrinth

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestExtendedWitnessAndInvalidRailsUseOrdinaryEnvironment(t *testing.T) {
	t.Parallel()
	for _, suite := range []Suite{SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder, SuiteRogue} {
		suite := suite
		t.Run(string(suite), func(t *testing.T) {
			t.Parallel()
			generated, err := GenerateExtended(ExtendedGeneratorConfig{
				Suite: suite, Seed: 81_000 + uint64(len(suite)),
				GeneratorVersion: ExtendedGeneratorVersionV1,
				GrammarVersion:   ExtendedGrammarVersionV1,
			})
			if err != nil {
				t.Fatal(err)
			}
			run, err := RunExtendedOracle(
				t.Context(), generated, cognition.EpisodeRef{ID: cognition.EpisodeID("episode-" + string(suite))},
				extendedTestActor(),
			)
			if err != nil {
				t.Fatal(err)
			}
			if !run.Terminal || len(run.Transitions) != len(generated.PrivateOracle().Witness)+1 {
				t.Fatalf("run=%#v", run)
			}
			if err := VerifyExtendedInvalidRails(
				t.Context(), generated, cognition.EpisodeRef{ID: cognition.EpisodeID("invalid-" + string(suite))},
				extendedTestActor(),
			); err != nil {
				t.Fatal(err)
			}
			if err := VerifyExtendedOmissionRails(
				t.Context(), generated,
				cognition.EpisodeRef{ID: cognition.EpisodeID("omission-" + string(suite))},
				extendedTestActor(),
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func extendedTestActor() cognition.AttemptRef {
	return cognition.AttemptRef{JobID: 41, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "extended-test"}
}
