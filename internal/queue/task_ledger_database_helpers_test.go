package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func insertTaskLedgerTestJob(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	marker string,
) (int64, string) {
	t.Helper()
	var runID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO omni_runs (status, prompt_summary)
		VALUES ('running', $1)
		RETURNING id::text
	`, marker).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	var jobID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO jobs (instruction, pipeline, status, metadata)
		VALUES ($1, $2, 'pending', jsonb_build_object('telemetry_run_id', $3::text))
		RETURNING id
	`, marker, model.PipelineCoding, runID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (job_id, generation, purpose) VALUES ($1, 1, 'initial')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	return jobID, runID
}

func insertTaskLedgerTestNode(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
	jobID int64,
	id string,
	objectiveID string,
	assignedStepID *int64,
	kind taskstate.NodeKind,
) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, objective_id, kind, title, status, priority,
			created_by, assigned_step_id, created_version, updated_version
		) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $3, 'pending', 1, 'code', $6, 1, 1)
	`, ledgerID, jobID, id, objectiveID, kind, assignedStepID); err != nil {
		t.Fatal(err)
	}
}

func expectTaskLedgerDatabaseFailure(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	statement string,
	arguments ...any,
) {
	t.Helper()
	const savepoint = "task_ledger_expected_failure"
	if _, err := tx.Exec(ctx, "SAVEPOINT "+savepoint); err != nil {
		t.Fatal(err)
	}
	_, operationErr := tx.Exec(ctx, statement, arguments...)
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); err != nil {
		t.Fatalf("recover expected PostgreSQL failure: %v (operation error: %v)", err, operationErr)
	}
	if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		t.Fatal(err)
	}
	if operationErr == nil {
		t.Fatalf("PostgreSQL accepted forbidden task ledger statement: %s", strings.TrimSpace(statement))
	}
}

func taskLedgerTestSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
