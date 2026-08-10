package cognitiongauntlet

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresOfflineHostRoleCanOnlyMutateHostThroughFencedAttempt(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run offline host role tests")
	}
	config := offlinePromotionTestConfig(t, databaseURL)
	database, err := prepareOfflinePromotionDatabase(
		t.Context(), config, loadRepositoryMigrationBundle(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupOfflinePromotionDatabase(t, database) })
	if err := database.enableHost(t.Context()); err != nil {
		t.Fatal(err)
	}
	restricted, err := promotionPool(
		t.Context(), database.hostURL, database.hostSchema+","+database.schema,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer restricted.Close()
	var currentRole string
	if err := restricted.QueryRow(t.Context(), `SELECT current_user`).Scan(&currentRole); err != nil {
		t.Fatal(err)
	}
	if currentRole != database.hostRole {
		t.Fatalf("restricted host role=%q want %q", currentRole, database.hostRole)
	}
	assertPermissionDenied(t, restricted, database.schema, "jobs")
	_, err = restricted.Exec(
		t.Context(), "UPDATE "+pgx.Identifier{database.schema, "jobs"}.Sanitize()+" SET updated_at=updated_at",
	)
	assertPostgresPermissionDenied(t, err, "restricted host queue DML")

	generated, err := labyrinth.Generate(labyrinth.GeneratorConfig{
		Suite: labyrinth.SuiteRetrieve, Seed: 991_001,
		Difficulty: labyrinth.Difficulty{
			WorldSize: 25, RelevantArtifacts: 3, SolutionDepth: 4,
			BranchingFactor: 1, DependencyCount: 2,
		},
		GeneratorVersion: labyrinth.GeneratorVersionV1,
		GrammarVersion:   labyrinth.GrammarVersionV1, SolverStateLimit: 100_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	scenario := generated.ExecutionScenario()
	episode := cognition.EpisodeRef{ID: cognition.EpisodeID("restricted-host-episode")}
	store, err := labyrinthhost.NewStoreInSchema(restricted, database.hostSchema)
	if err != nil {
		t.Fatal(err)
	}
	repository := queue.New(restricted)
	resolver := func(_ context.Context, ref cognition.ScenarioRef) (labyrinth.Scenario, error) {
		if ref != scenario.Ref() {
			return labyrinth.Scenario{}, cognition.ErrInvalidScenario
		}
		return scenario, nil
	}
	environment, err := labyrinthhost.NewEnvironment(
		store, episode, resolver, standaloneTransactionAuthorizer(restricted, repository),
		transactionAttemptAuthorizer(repository),
	)
	if err != nil {
		t.Fatal(err)
	}
	started, err := environment.Start(t.Context(), scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	action := restrictedHostWitnessAction(t, scenario, generated.PrivateOracle().Witness[0], started, database)
	transition, err := environment.Apply(t.Context(), episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	if transition.Current.Number != started.Current.Number+1 {
		t.Fatalf("restricted host revision=%d", transition.Current.Number)
	}
}

func restrictedHostWitnessAction(
	t *testing.T,
	scenario labyrinth.Scenario,
	witness labyrinth.WitnessAction,
	started cognition.Transition,
	database *offlinePromotionDatabase,
) cognition.RegisteredAction {
	t.Helper()
	schema, exists := scenario.Catalog().Schema(witness.Request.Kind)
	if !exists {
		t.Fatalf("witness schema %q is absent", witness.Request.Kind)
	}
	evidence := []cognition.EvidenceRef{}
	if schema.EvidencePolicy == cognition.EvidenceRequired {
		for _, observation := range started.Observations {
			evidence = append(evidence, observation.EvidenceRef())
		}
	}
	action, err := cognition.NewRegisteredAction(
		"restricted-host-action", attemptRefFromQueue(database), schema, witness.Request, evidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func attemptRefFromQueue(database *offlinePromotionDatabase) cognition.AttemptRef {
	return cognition.AttemptRef{
		JobID: database.attempt.JobID, Generation: database.attempt.Generation,
		StepID: database.attempt.StepID, Attempt: uint64(database.attempt.Attempt),
		WorkerID: database.attempt.WorkerID,
	}
}

func assertPostgresPermissionDenied(t *testing.T, err error, label string) {
	t.Helper()
	var postgres *pgconn.PgError
	if !errors.As(err, &postgres) || postgres.Code != "42501" {
		t.Fatalf("%s error=%v", label, err)
	}
}
