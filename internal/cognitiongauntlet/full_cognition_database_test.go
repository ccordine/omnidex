package cognitiongauntlet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresFullCognitionExecutesProductionRuntimeAndSealedTrace(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		ctx, fmt.Sprintf("gauntlet-full-cognition-%d", time.Now().UnixNano()),
		model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, fmt.Sprintf("gauntlet-worker-%d", job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claimed step=%+v, want job %d", claim, job.ID)
	}
	setBudget, err := fullCognitionWorkingSetBudget(fixture.Spec().Budget.WorkingSetBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCurrentWorkingSet(ctx, claim.Authority, setBudget); err != nil {
		t.Fatal(err)
	}
	generation := mustRatGeneration(t)
	fingerprint := transferTestFingerprint()
	fingerprint.ProductionSourceSHA256 = fullCognitionTestDigest(job.Instruction)
	client := &witnessPolicyClient{
		model:        generation.Fixed.Brain.Model,
		witness:      fixture.generated.PrivateOracle().Witness,
		evidenceUses: fixture.generated.PrivateOracle().EvidenceUses,
	}
	episodeDirectory, evaluationDirectory := t.TempDir(), t.TempDir()
	result, err := RunFullCognition(ctx, fixture, FullCognitionRunRequest{
		Surface: SurfaceSymbolic, RatGeneration: generation,
		RuntimeFingerprint: fingerprint, Repetition: 1,
		Attempt: claim.Authority, Pool: pool, Client: client, HostStore: hostStore,
		RestartAfterCycles:      []uint32{3},
		EpisodeSealPath:         filepath.Join(episodeDirectory, "episode.json"),
		EvaluationPath:          filepath.Join(evaluationDirectory, "evaluation.json"),
		LedgerSchemaVersion:     "task-ledger.v1",
		WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "context-projection.v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if !result.Evaluation.GoalSuccess || result.Variant.Variant != VariantFullCognition ||
		result.Episode.Manifest.Recovery.Restarts != 1 || client.calls() == 0 {
		t.Fatalf("full cognition result=%+v calls=%d", result, client.calls())
	}
	operationID, err := queue.NewLifecycleOperationID("gauntlet-test", fmt.Sprint(job.ID), "complete")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStep(ctx, queue.CompleteStepCommand{
		OperationID: operationID, Authority: claim.Authority, StepID: claim.Authority.StepID,
		Output: "Full cognition runtime integration test completed.",
	}); err != nil {
		t.Fatal(err)
	}
}

func openFullCognitionDatabase(
	t *testing.T,
) (context.Context, *pgxpool.Pool, *queue.Repository, *labyrinthhost.Store) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL full cognition tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("gauntlet_%d", time.Now().UnixNano())
	hostSchema := schema + "_host"
	identifier := pgx.Identifier{schema}.Sanitize()
	hostIdentifier := pgx.Identifier{hostSchema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Errorf("drop isolated gauntlet schema: %v", err)
		}
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+hostIdentifier+" CASCADE"); err != nil {
			t.Errorf("drop isolated Labyrinth host schema: %v", err)
		}
	})
	repository := queue.New(pool)
	if err := repository.EnsureSchema(ctx, loadRepositoryMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	hostStore, err := labyrinthhost.NewStoreInSchema(pool, hostSchema)
	if err != nil {
		t.Fatal(err)
	}
	if err := hostStore.InstallSchema(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, pool, repository, hostStore
}

func fullCognitionTestDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
