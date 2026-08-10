package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const fencedHostTestDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type fencedHostFixture struct {
	pool       *pgxpool.Pool
	repository *queue.Repository
	store      *Store
	scenario   labyrinth.Scenario
	witness    []labyrinth.WitnessAction
	episode    cognition.EpisodeRef
}

func newFencedHostFixture(t *testing.T) fencedHostFixture {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL Labyrinth host fencing tests")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	marker := time.Now().UnixNano()
	runtimeSchema := fmt.Sprintf("host_fence_%d", marker)
	hostSchema := runtimeSchema + "_world"
	runtimeID := pgx.Identifier{runtimeSchema}.Sanitize()
	hostID := pgx.Identifier{hostSchema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+runtimeID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+hostID); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = runtimeSchema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+runtimeID+" CASCADE")
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+hostID+" CASCADE")
	})
	t.Setenv("MIGRATIONS_DIR", filepath.Join("..", "..", "..", "migrations"))
	repository := queue.New(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreInSchema(pool, hostSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InstallSchema(ctx); err != nil {
		t.Fatal(err)
	}
	generated, err := labyrinth.Generate(labyrinth.GeneratorConfig{
		Suite: labyrinth.SuiteCombined, Seed: uint64(marker),
		Difficulty: labyrinth.Difficulty{
			WorldSize: 25, RelevantArtifacts: 3, SolutionDepth: 4,
			BranchingFactor: 1, DependencyCount: 2,
		},
		GeneratorVersion: labyrinth.GeneratorVersionV1,
		GrammarVersion:   labyrinth.GrammarVersionV1, SolverStateLimit: 10_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fencedHostFixture{
		pool: pool, repository: repository, store: store,
		scenario: generated.ExecutionScenario(), witness: generated.PrivateOracle().Witness,
		episode: cognition.EpisodeRef{ID: cognition.EpisodeID(fmt.Sprintf("fenced-host-%d", marker))},
	}
}

func (fixture fencedHostFixture) installExpiringAttempt(
	t *testing.T,
	worker string,
) model.StepAttemptAuthority {
	t.Helper()
	job, err := fixture.repository.EnqueueJob(
		t.Context(), "Labyrinth transactional lease fencing.", model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	var stepID int64
	if err := fixture.pool.QueryRow(t.Context(), `
		SELECT id FROM job_steps WHERE job_id=$1 ORDER BY sort_index,id LIMIT 1
	`, job.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	renewed := time.Now().UTC().Add(-queue.StepAttemptLeaseDuration + 750*time.Millisecond)
	if _, err := fixture.pool.Exec(t.Context(), `
		INSERT INTO job_step_attempts(
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES($1,1,$2,1,$3,$4,$4)
	`, job.ID, stepID, worker, renewed); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(t.Context(), `
		UPDATE job_steps SET status='running',worker_id=$2,current_attempt=1,
			started_at=$3,updated_at=$3 WHERE id=$1
	`, stepID, worker, renewed); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(t.Context(), `
		UPDATE jobs SET status='running',updated_at=$2 WHERE id=$1
	`, job.ID, renewed); err != nil {
		t.Fatal(err)
	}
	return model.StepAttemptAuthority{
		JobID: job.ID, Generation: 1, StepID: stepID, Attempt: 1, WorkerID: worker,
	}
}

func (fixture fencedHostFixture) installActiveAttempt(
	t *testing.T,
	worker string,
) model.StepAttemptAuthority {
	t.Helper()
	job, err := fixture.repository.EnqueueJob(
		t.Context(), "Labyrinth lifecycle fencing.", model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.repository.ClaimNextStep(t.Context(), worker)
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Authority.JobID != job.ID || claim.Authority.WorkerID != worker {
		t.Fatalf("initial production claim=%+v", claim)
	}
	if _, err := fixture.repository.CreateCurrentWorkingSet(
		t.Context(), claim.Authority,
		workingset.Budget{
			MaxItems: 32, MaxBytes: 128 * 1024,
			MaxPinnedItems: 16, MaxPinnedBytes: 96 * 1024,
		},
	); err != nil {
		t.Fatal(err)
	}
	return claim.Authority
}

func (fixture fencedHostFixture) newEnvironment(
	t *testing.T,
	precheck labyrinth.AttemptAuthorizer,
	transactional TransactionAttemptAuthorizer,
) *Environment {
	t.Helper()
	resolver := func(_ context.Context, reference cognition.ScenarioRef) (labyrinth.Scenario, error) {
		if reference != fixture.scenario.Ref() {
			return labyrinth.Scenario{}, cognition.ErrInvalidScenario
		}
		return fixture.scenario, nil
	}
	environment, err := NewEnvironment(
		fixture.store, fixture.episode, resolver, precheck, transactional,
	)
	if err != nil {
		t.Fatal(err)
	}
	return environment
}

func (fixture fencedHostFixture) startCognitionEpisode(
	t *testing.T,
	authority model.StepAttemptAuthority,
	started cognition.Transition,
) {
	t.Helper()
	goal := fixture.scenario.Goal()
	completion, err := labyrinth.NewCompletionAuthority(fixture.scenario)
	if err != nil {
		t.Fatal(err)
	}
	check, err := completion.Resolve(goal)
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := cognition.DeriveObligationID(
		fixture.episode.ID, cognition.InitialObligationGeneration, "", goal, check,
	)
	if err != nil {
		t.Fatal(err)
	}
	brain := fencedHostAttestedBrain(t)
	budget := cognition.RuntimeBudget{
		RemainingPolicyCalls: 4, MaxInputBytes: 8_192, MaxInputTokens: 2_048,
		MaxOutputBytes: 4_096, MaxOutputTokens: 256, MaxEvidenceRefs: 16,
		MaxActionArguments: 16, MaxLedgerProposals: 4, MaxAttentionRequests: 4,
		MaxExpectedEffectBytes: 1_024,
	}
	if _, err := fixture.repository.StartCognitionEpisode(
		t.Context(), queue.CognitionEpisodeStart{
			Authority: authority, EpisodeID: fixture.episode.ID, AttestedBrain: brain,
			Scenario: fixture.scenario.Ref(), Goal: goal, Completion: completion,
			ActionCatalog: fixture.scenario.Catalog(), Budget: budget,
			Root: cognition.ObligationSpec{
				ID: rootID, Desired: goal, DependsOn: []cognition.ObligationID{},
				SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
			},
			Transition: started,
		}, cognitionstate.NewNoFactAcceptanceAuthority(),
	); err != nil {
		t.Fatal(err)
	}
}
