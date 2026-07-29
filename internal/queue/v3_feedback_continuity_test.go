package queue

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestV3CodingFeedbackRequeuesTheSameJob(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL feedback continuity test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	repo := New(pool)
	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	location := fmt.Sprintf("/tmp/omnidex-feedback-%d", time.Now().UnixNano())
	job, err := repo.EnqueueJob(ctx, "Build the application", model.PipelineCoding, []byte(fmt.Sprintf(`{"client_cwd":%q}`, location)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM omni_runs WHERE id = NULLIF((SELECT metadata->>'telemetry_run_id' FROM jobs WHERE id = $1), '')::uuid`, job.ID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM jobs WHERE id = $1`, job.ID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM projects WHERE location = $1`, location)
	})

	details, err := repo.GetJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Steps) != 1 || details.Steps[0].Action != "v3_coding" {
		t.Fatalf("coding steps=%+v", details.Steps)
	}
	stepID := details.Steps[0].ID
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status = $2 WHERE id = $1`, job.ID, model.JobStatusRunning); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_steps SET status = $2, worker_id = 'test-worker' WHERE id = $1`, stepID, model.StepStatusRunning); err != nil {
		t.Fatal(err)
	}

	controlled, err := repo.InterruptJob(ctx, job.ID, "Correct the current CLI file; keep completed domain code")
	if err != nil {
		t.Fatal(err)
	}
	if controlled.ID != job.ID {
		t.Fatalf("feedback created successor job %d from %d", controlled.ID, job.ID)
	}
	details, err = repo.GetJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Steps[0].Status != model.StepStatusPending {
		t.Fatalf("coding step status=%s want pending", details.Steps[0].Status)
	}
	found := false
	for _, item := range details.Contexts {
		if item.StepID == stepID && item.Key == "user_feedback" && strings.Contains(item.Value, "current CLI file") {
			found = true
		}
	}
	if !found {
		t.Fatalf("direct feedback did not reach the same coding step: %+v", details.Contexts)
	}
}
