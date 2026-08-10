package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func claimReplannedRepositoryCognitionJob(
	t *testing.T,
	ctx context.Context,
	repository *queue.Repository,
	projectID int64,
	root string,
) *model.ClaimedStep {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{"project_id": projectID, "client_cwd": root})
	if err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("repository-cognition-generation-two-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, metadata)
	if err != nil {
		t.Fatal(err)
	}
	cancelID, err := queue.NewLifecycleOperationID("repository-cognition-cleanup", fmt.Sprint(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = repository.CancelJob(context.Background(), queue.CancelJobCommand{
			OperationID: cancelID, JobID: job.ID, Reason: "close repository cognition proof",
		})
	})
	replanID, err := queue.NewLifecycleOperationID("repository-cognition-generation-two", fmt.Sprint(job.ID))
	if err != nil {
		t.Fatal(err)
	}
	replanned, err := repository.ReplanJob(ctx, queue.ReplanJobCommand{
		OperationID: replanID, JobID: job.ID,
		Feedback: "Exercise the same accepted repository investigation under fresh job authority.",
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if replanned.CurrentGeneration != 2 || claim == nil || claim.Job.ID != job.ID ||
		claim.Authority.Generation != 2 {
		t.Fatalf("replanned job=%+v claim=%+v", replanned, claim)
	}
	return claim
}

func openRepositoryCognitionDatabase(
	t *testing.T,
) (context.Context, *queue.Repository, *pgxpool.Pool) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL repository cognition tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("repository_cognition_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = admin.Exec(cleanup, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	t.Setenv("MIGRATIONS_DIR", "../../migrations")
	repository := queue.New(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, repository, pool
}

type repositoryCognitionCounts struct {
	policyCalls, actions, transitions, seals, mutations int
}

func loadRepositoryCognitionCounts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	episodeID string,
	jobID int64,
) repositoryCognitionCounts {
	t.Helper()
	var counts repositoryCognitionCounts
	err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_transitions WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM repository_mutation_operations WHERE job_id=$2)
	`, episodeID, jobID).Scan(
		&counts.policyCalls, &counts.actions, &counts.transitions,
		&counts.seals, &counts.mutations,
	)
	if err != nil {
		t.Fatal(err)
	}
	return counts
}
