package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresJobEnqueueCreatesInitialTaskAuthorityAtomically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}

	marker := fmt.Sprintf("task-ledger-enqueue-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker+"-public", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	assertEnqueuedTaskLedger(t, ctx, pool, job.ID)
	assertInitialTaskLedgerRead(t, ctx, repository, job.ID)

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	nestedJob, err := repository.enqueueJobTx(
		ctx, tx, marker+"-nested", model.PipelineCoding, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	assertEnqueuedTaskLedger(t, ctx, tx, nestedJob.ID)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertInitialTaskLedgerRead(t, ctx, repository, nestedJob.ID)

	failureMarker := marker + "-forced-ledger-failure"
	installTaskLedgerFailureTrigger(t, ctx, pool, failureMarker)
	if _, err := repository.EnqueueJob(ctx, failureMarker, model.PipelineCoding, []byte(`{}`)); err == nil ||
		!strings.Contains(err.Error(), "create task ledger") {
		t.Fatalf("forced task ledger failure error=%v", err)
	}
	var jobs, runs int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE instruction=$1`, failureMarker).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM omni_runs WHERE prompt_summary=$1`, failureMarker).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || runs != 0 {
		t.Fatalf("failed ledger enqueue committed jobs=%d runs=%d", jobs, runs)
	}
}

type taskLedgerBindingQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func assertEnqueuedTaskLedger(
	t *testing.T,
	ctx context.Context,
	queryer taskLedgerBindingQueryer,
	jobID int64,
) {
	t.Helper()
	var runID, ledgerID, ledgerRunID, ownerType, status string
	var ownerID, version, nodes, edges, entries, events int64
	if err := queryer.QueryRow(ctx, `
		SELECT jobs.metadata ->> 'telemetry_run_id', ledger.id, ledger.run_id::text,
		       ledger.owner_type, ledger.owner_id, ledger.version, ledger.status,
		       (SELECT COUNT(*) FROM task_nodes WHERE ledger_id=ledger.id),
		       (SELECT COUNT(*) FROM task_node_edges WHERE ledger_id=ledger.id),
		       (SELECT COUNT(*) FROM task_entries WHERE ledger_id=ledger.id),
		       (SELECT COUNT(*) FROM task_events WHERE ledger_id=ledger.id)
		FROM jobs
		JOIN task_ledgers ledger ON ledger.job_id=jobs.id
		WHERE jobs.id=$1
	`, jobID).Scan(
		&runID, &ledgerID, &ledgerRunID, &ownerType, &ownerID, &version, &status,
		&nodes, &edges, &entries, &events,
	); err != nil {
		t.Fatal(err)
	}
	owner := taskstate.LedgerOwner{Kind: taskstate.OwnerJob, JobID: jobID, RunID: runID}
	wantID, err := taskstate.NewLedgerID(owner)
	if err != nil {
		t.Fatal(err)
	}
	if ledgerID != string(wantID) || ledgerRunID != runID || ownerType != string(taskstate.OwnerJob) ||
		ownerID != jobID || version != int64(initialTaskLedgerVersion) || status != string(taskstate.LedgerActive) ||
		nodes != 1 || edges != 0 || entries != 1 || events != int64(initialTaskLedgerVersion) {
		t.Fatalf(
			"ledger binding id=%q run=%q/%q owner=%q/%d version=%d status=%q contents=%d/%d/%d/%d",
			ledgerID, ledgerRunID, runID, ownerType, ownerID, version, status, nodes, edges, entries, events,
		)
	}
}

func assertInitialTaskLedgerRead(t *testing.T, ctx context.Context, repository *Repository, jobID int64) {
	t.Helper()
	state, err := repository.TaskLedger(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Version != initialTaskLedgerVersion || state.Status != taskstate.LedgerActive || state.Owner.JobID != jobID ||
		len(state.Nodes) != 1 || len(state.Edges) != 0 || len(state.Entries) != 1 {
		t.Fatalf("enqueued task ledger has invalid initial authority: %+v", state)
	}
}

func installTaskLedgerFailureTrigger(t *testing.T, ctx context.Context, pool *pgxpool.Pool, marker string) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	functionName := pgx.Identifier{"task_ledger_enqueue_failure_" + suffix}.Sanitize()
	triggerName := pgx.Identifier{"task_ledger_enqueue_failure_trigger_" + suffix}.Sanitize()
	markerLiteral := "'" + strings.ReplaceAll(marker, "'", "''") + "'"
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			IF EXISTS (SELECT 1 FROM jobs WHERE id=NEW.job_id AND instruction=%s) THEN
				RAISE EXCEPTION 'forced task ledger enqueue failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER %s BEFORE INSERT ON task_ledgers
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, functionName, markerLiteral, triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, fmt.Sprintf(
			"DROP TRIGGER %s ON task_ledgers; DROP FUNCTION %s()", triggerName, functionName,
		)); err != nil {
			t.Errorf("remove task ledger failure trigger: %v", err)
		}
	})
}
