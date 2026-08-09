package queue

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStaleV3LeaseFailsWithoutReusingStepIdentity(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL stale-step lease test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := New(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	insertRunningStep := func(action string, updatedAt time.Time) int64 {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		var jobID, stepID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO jobs (instruction, pipeline, status, metadata, updated_at)
			VALUES ('stale lease test', $1, $2, '{}'::jsonb, $3)
			RETURNING id
		`, model.PipelineAssistant, model.JobStatusRunning, updatedAt).Scan(&jobID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_generations (job_id, generation, purpose) VALUES ($1, 1, 'initial')
		`, jobID); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO job_steps (
				job_id, action, sort_index, status, worker_id, started_at, updated_at, generation
			) VALUES ($1, $2, 1, $3, 'dead-worker', $4, $4, 1)
			RETURNING id
		`, jobID, action, model.StepStatusRunning, updatedAt).Scan(&stepID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return stepID
	}

	now := time.Now().UTC()
	staleV3 := insertRunningStep("v3_subtask", now.Add(-3*time.Minute))
	freshV3 := insertRunningStep("v3_subtask", now)
	staleLegacy := insertRunningStep("legacy_execute", now.Add(-3*time.Minute))

	err = repository.CheckStaleV3StepLeases(ctx, now.Add(-time.Minute))
	if !errors.Is(err, ErrStepLeaseRequired) || !strings.Contains(err.Error(), "automatic identity reuse is forbidden") {
		t.Fatalf("stale V3 lease error=%v", err)
	}
	for _, stepID := range []int64{staleV3, freshV3, staleLegacy} {
		var status, worker string
		var startedAt *time.Time
		if err := pool.QueryRow(ctx, `
			SELECT status, COALESCE(worker_id, ''), started_at FROM job_steps WHERE id=$1
		`, stepID).Scan(&status, &worker, &startedAt); err != nil {
			t.Fatal(err)
		}
		if status != model.StepStatusRunning || worker != "dead-worker" || startedAt == nil {
			t.Fatalf("stale check mutated step %d: status=%q worker=%q started=%v", stepID, status, worker, startedAt)
		}
	}
}
