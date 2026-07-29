package queue

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecoverStaleV3StepsRequeuesOnlyExpiredV3Leases(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL stale-step recovery test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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

	jobIDs := make([]int64, 0, 3)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, jobID := range jobIDs {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM jobs WHERE id = $1`, jobID)
		}
	})

	insertRunningStep := func(action string, updatedAt time.Time) (int64, int64) {
		t.Helper()
		var jobID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO jobs (instruction, pipeline, status, metadata, updated_at)
			VALUES ('recovery test', $1, $2, '{}'::jsonb, $3)
			RETURNING id
		`, model.PipelineAssistant, model.JobStatusRunning, updatedAt).Scan(&jobID); err != nil {
			t.Fatal(err)
		}
		jobIDs = append(jobIDs, jobID)
		var stepID int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO job_steps (job_id, action, sort_index, status, worker_id, started_at, updated_at)
			VALUES ($1, $2, 1, $3, 'dead-worker', $4, $4)
			RETURNING id
		`, jobID, action, model.StepStatusRunning, updatedAt).Scan(&stepID); err != nil {
			t.Fatal(err)
		}
		return jobID, stepID
	}

	now := time.Now().UTC()
	_, staleV3 := insertRunningStep("v3_subtask", now.Add(-3*time.Minute))
	_, freshV3 := insertRunningStep("v3_subtask", now)
	_, staleLegacy := insertRunningStep("legacy_execute", now.Add(-3*time.Minute))

	recovered, err := repo.RecoverStaleV3Steps(ctx, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if recovered < 1 {
		t.Fatalf("RecoverStaleV3Steps()=%d, want at least the test lease", recovered)
	}

	assertState := func(stepID int64, wantStatus, wantWorker string, wantStarted bool) {
		t.Helper()
		var status, worker string
		var startedAt *time.Time
		if err := pool.QueryRow(ctx, `
			SELECT status, COALESCE(worker_id, ''), started_at
			FROM job_steps
			WHERE id = $1
		`, stepID).Scan(&status, &worker, &startedAt); err != nil {
			t.Fatal(err)
		}
		if status != wantStatus || worker != wantWorker || (startedAt != nil) != wantStarted {
			t.Fatalf("step %d state=(%s,%q,started=%t), want (%s,%q,started=%t)", stepID, status, worker, startedAt != nil, wantStatus, wantWorker, wantStarted)
		}
	}
	assertState(staleV3, model.StepStatusPending, "", false)
	assertState(freshV3, model.StepStatusRunning, "dead-worker", true)
	assertState(staleLegacy, model.StepStatusRunning, "dead-worker", true)
}
