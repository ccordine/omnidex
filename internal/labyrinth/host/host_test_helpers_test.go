package host

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	generatedOnce sync.Once
	generatedCase labyrinth.GeneratedCase
	generatedErr  error
)

type durableFixture struct {
	Pool     *pgxpool.Pool
	Store    *Store
	Scenario labyrinth.Scenario
	Witness  []labyrinth.WitnessAction
	Episode  cognition.EpisodeRef
	Actor    cognition.AttemptRef
}

func newDurableFixture(t *testing.T) durableFixture {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL Labyrinth host tests")
	}
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InstallSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	generatedOnce.Do(func() {
		generatedCase, generatedErr = labyrinth.Generate(labyrinth.GeneratorConfig{
			Suite: labyrinth.SuiteCombined, Seed: 94721,
			Difficulty: labyrinth.Difficulty{
				WorldSize: 25, RelevantArtifacts: 3, SolutionDepth: 4,
				BranchingFactor: 1, DependencyCount: 2,
			},
			GeneratorVersion: labyrinth.GeneratorVersionV1,
			GrammarVersion:   labyrinth.GrammarVersionV1,
			SolverStateLimit: 10_000,
		})
	})
	if generatedErr != nil {
		t.Fatal(generatedErr)
	}
	marker := time.Now().UnixNano()
	episode := cognition.EpisodeRef{ID: cognition.EpisodeID(fmt.Sprintf("durable-host-%d", marker))}
	fixture := durableFixture{
		Pool: pool, Store: store, Scenario: generatedCase.ExecutionScenario(),
		Witness: generatedCase.PrivateOracle().Witness, Episode: episode,
		Actor: cognition.AttemptRef{
			JobID: marker, Generation: 1, StepID: 1, Attempt: 1,
			WorkerID: fmt.Sprintf("durable-worker-%d", marker),
		},
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM labyrinth_host.action_receipts WHERE episode_id=$1`, episode.ID)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM labyrinth_host.episodes WHERE episode_id=$1`, episode.ID)
	})
	return fixture
}

func (fixture durableFixture) resolver() ScenarioResolver {
	return func(_ context.Context, reference cognition.ScenarioRef) (labyrinth.Scenario, error) {
		if reference != fixture.Scenario.Ref() {
			return labyrinth.Scenario{}, cognition.ErrInvalidScenario
		}
		return fixture.Scenario, nil
	}
}

func (fixture durableFixture) environment(
	t *testing.T,
	authorized func(cognition.AttemptRef) bool,
) *Environment {
	t.Helper()
	environment, err := NewEnvironment(
		fixture.Store, fixture.Episode, fixture.resolver(),
		func(_ context.Context, actor cognition.AttemptRef) error {
			if !authorized(actor) {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
		func(_ context.Context, _ pgx.Tx, actor cognition.AttemptRef) error {
			if !authorized(actor) {
				return cognition.ErrAuthorityDenied
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return environment
}

func (fixture durableFixture) action(
	t *testing.T,
	index int,
	id cognition.ActionID,
	actor cognition.AttemptRef,
) cognition.RegisteredAction {
	t.Helper()
	witness := fixture.Witness[index]
	schema, exists := fixture.Scenario.Catalog().Schema(witness.Request.Kind)
	if !exists {
		t.Fatalf("witness schema %q is absent", witness.Request.Kind)
	}
	action, err := cognition.NewRegisteredAction(id, actor, schema, witness.Request, nil)
	if err != nil {
		t.Fatal(err)
	}
	return action
}
