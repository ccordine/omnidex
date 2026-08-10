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

	details, err := repo.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Steps) != 1 || details.Steps[0].Action != "v3_coding" {
		t.Fatalf("coding steps=%+v", details.Steps)
	}
	stepID := details.Steps[0].ID
	activateStepAttemptForTest(t, ctx, pool, job.ID, 1, stepID, "test-worker")

	controlled, err := repo.InterruptJob(ctx, testReplanCommand(
		t, job.ID, "v3-feedback", "Correct the current CLI file; keep completed domain code",
	))
	if err != nil {
		t.Fatal(err)
	}
	if controlled.ID != job.ID {
		t.Fatalf("feedback created successor job %d from %d", controlled.ID, job.ID)
	}
	details, err = repo.CurrentJobDetails(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if controlled.CurrentGeneration != 2 || details.Job.CurrentGeneration != 2 {
		t.Fatalf("interrupt generation controlled=%d details=%d", controlled.CurrentGeneration, details.Job.CurrentGeneration)
	}
	if len(details.Steps) != 1 || details.Steps[0].ID == stepID ||
		details.Steps[0].Generation != 2 || details.Steps[0].Status != model.StepStatusPending {
		t.Fatalf("current replacement step=%+v, retired=%d", details.Steps, stepID)
	}
	for _, item := range details.Contexts {
		if item.StepID == stepID && item.Key == "user_feedback" && strings.Contains(item.Value, "current CLI file") {
			t.Fatalf("interrupt leaked feedback into retired step context: %+v", item)
		}
	}
	var oldStatus string
	var supersededAt *int64
	if err := pool.QueryRow(ctx, `
		SELECT status, superseded_at_generation FROM job_steps WHERE id=$1
	`, stepID).Scan(&oldStatus, &supersededAt); err != nil {
		t.Fatal(err)
	}
	if oldStatus != model.StepStatusCanceled || supersededAt == nil || *supersededAt != 2 {
		t.Fatalf("retired step status=%q superseded_at=%v", oldStatus, supersededAt)
	}
	var storedFeedback string
	if err := pool.QueryRow(ctx, `
		SELECT feedback FROM job_generations WHERE job_id=$1 AND generation=2
	`, job.ID).Scan(&storedFeedback); err != nil {
		t.Fatal(err)
	}
	if storedFeedback != "Correct the current CLI file; keep completed domain code" {
		t.Fatalf("generation feedback=%q", storedFeedback)
	}
}
