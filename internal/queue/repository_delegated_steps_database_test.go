package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresDelegatedExpansionRejectsSupersededAnchor(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	job, anchorStepID, _ := runningPlanningJob(t, repository, pool, ctx, "stale-anchor")
	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "retire-observed", "Retire the observed planning generation.")); err != nil {
		t.Fatal(err)
	}

	_, err := repository.ExpandDelegatedSubtasks(
		ctx, job.ID, anchorStepID, []artifacts.Subtask{delegatedSubtaskFixture("stale-task")},
	)
	if !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatalf("superseded anchor error=%v", err)
	}
	var generation, delegated int64
	if err := pool.QueryRow(ctx, `
		SELECT jobs.current_generation,
		       (SELECT COUNT(*) FROM job_steps
		        WHERE job_id=jobs.id AND action=$2)
		FROM jobs WHERE id=$1
	`, job.ID, delegatedSubtaskAction).Scan(&generation, &delegated); err != nil {
		t.Fatal(err)
	}
	if generation != 2 || delegated != 0 {
		t.Fatalf("rejected stale expansion mutated generation=%d delegated=%d", generation, delegated)
	}
}

func TestPostgresDelegatedExpansionSerializesBeforeReplan(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	job, anchorStepID, tailStepID := runningPlanningJob(t, repository, pool, ctx, "lock-order")

	blocker, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	blockerOpen := true
	defer func() {
		if blockerOpen {
			_ = blocker.Rollback(context.Background())
		}
	}()
	var lockedTail int64
	if err := blocker.QueryRow(ctx, `
		SELECT id FROM job_steps WHERE id=$1 FOR UPDATE
	`, tailStepID).Scan(&lockedTail); err != nil {
		t.Fatal(err)
	}

	expandName := fmt.Sprintf("omni_expand_%d", time.Now().UnixNano())
	replanName := fmt.Sprintf("omni_replan_%d", time.Now().UnixNano())
	expandRepository, expandPool := namedRepository(t, ctx, expandName)
	replanRepository, replanPool := namedRepository(t, ctx, replanName)
	defer expandPool.Close()
	defer replanPool.Close()

	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	expandResult := make(chan error, 1)
	go func() {
		_, callErr := expandRepository.ExpandDelegatedSubtasks(
			runCtx, job.ID, anchorStepID,
			[]artifacts.Subtask{delegatedSubtaskFixture("concurrent-task")},
		)
		expandResult <- callErr
	}()
	if err := waitForNamedLock(runCtx, pool, expandName); err != nil {
		_ = blocker.Rollback(context.Background())
		blockerOpen = false
		t.Fatal(err)
	}

	replanResult := make(chan error, 1)
	go func() {
		_, callErr := replanRepository.ReplanJob(runCtx, testReplanCommand(
			t, job.ID, "serialize-delegated", "Serialize replanning after delegated expansion.",
		))
		replanResult <- callErr
	}()
	if err := waitForNamedLock(runCtx, pool, replanName); err != nil {
		_ = blocker.Rollback(context.Background())
		blockerOpen = false
		t.Fatal(err)
	}
	if err := blocker.Commit(ctx); err != nil {
		blockerOpen = false
		t.Fatal(err)
	}
	blockerOpen = false

	if err := receiveConcurrentResult(runCtx, expandResult, "delegated expansion"); err != nil {
		t.Fatal(err)
	}
	if err := receiveConcurrentResult(runCtx, replanResult, "replan"); err != nil {
		t.Fatal(err)
	}

	var currentGeneration, currentDelegated, historicalDelegated int64
	if err := pool.QueryRow(ctx, `
		SELECT jobs.current_generation,
		       COUNT(*) FILTER (
		           WHERE steps.action=$2 AND steps.superseded_at_generation IS NULL
		       ),
		       COUNT(*) FILTER (WHERE steps.action=$2)
		FROM jobs
		JOIN job_steps AS steps ON steps.job_id=jobs.id
		WHERE jobs.id=$1
		GROUP BY jobs.current_generation
	`, job.ID, delegatedSubtaskAction).Scan(
		&currentGeneration, &currentDelegated, &historicalDelegated,
	); err != nil {
		t.Fatal(err)
	}
	if currentGeneration != 2 || currentDelegated != 0 || historicalDelegated != 1 {
		t.Fatalf(
			"serialized state generation=%d current delegated=%d historical delegated=%d",
			currentGeneration, currentDelegated, historicalDelegated,
		)
	}
}

func runningPlanningJob(
	t *testing.T,
	repository *Repository,
	pool *pgxpool.Pool,
	ctx context.Context,
	label string,
) (model.Job, int64, int64) {
	t.Helper()
	job, err := repository.EnqueueJob(
		ctx, fmt.Sprintf("delegated-%s-%d", label, time.Now().UnixNano()),
		model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	var anchorStepID, tailStepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps
		WHERE job_id=$1 AND action=$2 AND generation=1
	`, job.ID, replanPlanningBoundary).Scan(&anchorStepID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps
		WHERE job_id=$1 AND action='v3_analysis' AND generation=1
	`, job.ID).Scan(&tailStepID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_steps
		SET status=CASE
		    WHEN id=$2 THEN $3
		    WHEN sort_index < 40 THEN $4
		    ELSE status
		END,
		updated_at=NOW()
		WHERE job_id=$1 AND generation=1
	`, job.ID, anchorStepID, model.StepStatusRunning, model.StepStatusCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE jobs SET status=$2, updated_at=NOW() WHERE id=$1
	`, job.ID, model.JobStatusRunning); err != nil {
		t.Fatal(err)
	}
	return job, anchorStepID, tailStepID
}

func namedRepository(
	t *testing.T,
	ctx context.Context,
	applicationName string,
) (*Repository, *pgxpool.Pool) {
	t.Helper()
	config, err := pgxpool.ParseConfig(strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL")))
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	config.ConnConfig.RuntimeParams["application_name"] = applicationName
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	return New(pool), pool
}

func waitForNamedLock(ctx context.Context, pool *pgxpool.Pool, applicationName string) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waitType string
		err := pool.QueryRow(ctx, `
			SELECT COALESCE(wait_event_type, '')
			FROM pg_stat_activity
			WHERE application_name=$1
			ORDER BY backend_start DESC
			LIMIT 1
		`, applicationName).Scan(&waitType)
		if err == nil && waitType == "Lock" {
			return nil
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("inspect %s lock wait: %w", applicationName, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %s lock: %w", applicationName, ctx.Err())
		case <-ticker.C:
		}
	}
}

func receiveConcurrentResult(ctx context.Context, result <-chan error, operation string) error {
	select {
	case err := <-result:
		if err != nil {
			return fmt.Errorf("%s failed: %w", operation, err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s did not finish: %w", operation, ctx.Err())
	}
}
