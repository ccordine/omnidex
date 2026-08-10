package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresTaskLedgerSchemaEnforcesTypedAuthority(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL task ledger tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())

	marker := fmt.Sprintf("task-ledger-schema-%d", time.Now().UnixNano())
	jobID, runID := insertTaskLedgerTestJob(t, ctx, tx, marker+"-one")
	otherJobID, _ := insertTaskLedgerTestJob(t, ctx, tx, marker+"-two")
	var stepID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO job_steps (job_id, action, sort_index, status, generation)
		VALUES ($1, 'task_ledger_test', 0, 'pending', 1)
		RETURNING id
	`, jobID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	owner := taskstate.LedgerOwner{Kind: taskstate.OwnerJob, JobID: jobID, RunID: runID}
	ledgerID, err := taskstate.NewLedgerID(owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_ledgers (id, job_id, run_id, owner_type, owner_id, version)
		VALUES ($1, $2, $3::uuid, $4, $2, 1)
	`, ledgerID, jobID, runID, taskstate.OwnerJob); err != nil {
		t.Fatal(err)
	}

	insertTaskLedgerTestNode(t, ctx, tx, ledgerID, jobID, "objective", "", nil, taskstate.NodeObjective)
	insertTaskLedgerTestNode(t, ctx, tx, ledgerID, jobID, "first", "objective", &stepID, taskstate.NodeTask)
	insertTaskLedgerTestNode(t, ctx, tx, ledgerID, jobID, "second", "objective", nil, taskstate.NodeTask)
	var objectiveChildren int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM task_nodes WHERE ledger_id=$1 AND objective_id='objective'
	`, ledgerID).Scan(&objectiveChildren); err != nil {
		t.Fatal(err)
	}
	if objectiveChildren != 2 {
		t.Fatalf("objective child count=%d, want 2", objectiveChildren)
	}

	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, kind, title, status, priority, created_by,
			created_version, updated_version
		) VALUES ($1, $2, 'cross-job', 'task', 'Cross job', 'pending', 1, 'code', 1, 1)
	`, ledgerID, otherJobID)
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, objective_id, kind, title, status, priority,
			created_by, assigned_step_id, created_version, updated_version
		) VALUES ($1, $2, 'duplicate-step', 'objective', 'task', 'Duplicate step',
			'pending', 1, 'code', $3, 1, 1)
	`, ledgerID, jobID, stepID)
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, objective_id, kind, title, status, priority,
			created_by, created_version, updated_version
		) VALUES ($1, $2, 'self-objective', 'self-objective', 'task', 'Self objective',
			'pending', 1, 'code', 1, 1)
	`, ledgerID, jobID)
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, kind, title, status, priority,
			created_by, created_version, updated_version
		) VALUES ($1, $2, U&'\00A0unicode-space', 'task', 'Unicode space',
			'pending', 1, 'code', 1, 1)
	`, ledgerID, jobID)
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, kind, title, status, priority,
			created_by, acceptance_criteria, created_version, updated_version
		) VALUES ($1, $2, 'null-criteria', 'task', 'Null criteria',
			'pending', 1, 'code', 'null'::jsonb, 1, 1)
	`, ledgerID, jobID)
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, kind, title, status, priority,
			created_by, metadata, created_version, updated_version
		) VALUES ($1, $2, 'null-node-metadata', 'task', 'Null metadata',
			'pending', 1, 'code', 'null'::jsonb, 1, 1)
	`, ledgerID, jobID)
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, kind, title, status, priority,
			created_by, metadata, created_version, updated_version
		) VALUES ($1, $2, 'oversized-node-metadata', 'task', 'Oversized metadata',
			'pending', 1, 'code', jsonb_build_object('blob', repeat('x', 131072)), 1, 1)
	`, ledgerID, jobID)

	feedback := "Replan using the accepted correction."
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_entries (
			ledger_id, job_id, id, kind, feedback_purpose, status, authority, content,
			content_sha256, created_by, created_version, updated_version
		) VALUES ($1, $2, 'feedback', 'feedback', 'replan', 'active', 'user',
			$3, $4, 'user', 1, 1)
	`, ledgerID, jobID, feedback, taskLedgerTestSHA256(feedback)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_entry_refs (
			ledger_id, job_id, entry_id, uri, version, content_sha256, relation, position
		) VALUES ($1, $2, 'feedback', 'evidence://feedback/1', '1', $3, 'evidence', 0)
	`, ledgerID, jobID, strings.Repeat("d", 64)); err != nil {
		t.Fatal(err)
	}
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_entry_refs (
			ledger_id, job_id, entry_id, uri, version, content_sha256, relation, position
		) VALUES ($1, $2, 'feedback', 'missing-scheme', '1', $3, 'evidence', 1)
	`, ledgerID, jobID, strings.Repeat("d", 64))
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_entry_refs (
			ledger_id, job_id, entry_id, uri, version, content_sha256, relation, position
		) VALUES ($1, $2, 'feedback', 'evidence://feedback/cross-job', '1', $3, 'evidence', 1)
	`, ledgerID, otherJobID, strings.Repeat("e", 64))
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_entries (
			ledger_id, job_id, id, kind, status, authority, content, content_sha256,
			created_by, created_version, updated_version
		) VALUES ($1, $2, 'untyped-feedback', 'feedback', 'active', 'user',
			'Untyped', $3, 'user', 1, 1)
	`, ledgerID, jobID, taskLedgerTestSHA256("Untyped"))
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_entries (
			ledger_id, job_id, id, kind, status, authority, content, content_sha256,
			created_by, created_version, updated_version
		) VALUES ($1, $2, 'wrong-hash', 'note', 'active', 'code',
			'Exact content', $3, 'code', 1, 1)
	`, ledgerID, jobID, strings.Repeat("f", 64))
	oversizedEntryContent := "Oversized entry metadata"
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_entries (
			ledger_id, job_id, id, kind, status, authority, content, content_sha256,
			created_by, metadata, created_version, updated_version
		) VALUES ($1, $2, 'oversized-entry-metadata', 'note', 'active', 'code',
			$3, $4, 'code', jsonb_build_object('blob', repeat('x', 131072)), 1, 1)
	`, ledgerID, jobID, oversizedEntryContent, taskLedgerTestSHA256(oversizedEntryContent))
	badDispositionContent := "Forbidden disposition actor"
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_entries (
			ledger_id, job_id, id, kind, status, authority, content, content_sha256,
			created_by, disposition_reason, disposition_by, created_version, updated_version
		) VALUES ($1, $2, 'bad-disposition-actor', 'note', 'resolved', 'code',
			$3, $4, 'code', 'Resolved by evidence', 'tool_evidence', 1, 1)
	`, ledgerID, jobID, badDispositionContent, taskLedgerTestSHA256(badDispositionContent))

	commandID, err := taskstate.NewCommandID(marker, "event-one")
	if err != nil {
		t.Fatal(err)
	}
	commandSHA := strings.Repeat("a", 64)
	event := taskstate.Event{
		LedgerID: ledgerID, Version: 1, CommandID: commandID,
		CommandSHA256: commandSHA, CommandKind: taskstate.CommandAddNode,
		Kind: taskstate.EventNodeAdded, Authority: taskstate.AuthorityCode,
		Node: &taskstate.Node{
			ID: "objective", Kind: taskstate.NodeObjective, Title: "objective",
			Status: taskstate.NodePending, Priority: 1, CreatedBy: taskstate.AuthorityCode,
			VerificationRefs: []taskstate.Ref{}, AcceptanceCriteria: []string{},
			Metadata:       taskstate.EmptyJSONObject(),
			CreatedVersion: 1, UpdatedVersion: 1,
		},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var eventID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO task_events (
			ledger_id, job_id, job_generation, ledger_version, command_id, command_sha256,
			command_kind, event_kind, actor, payload
		) VALUES ($1, $2, 1, 1, $3, $4, $5, $6, $7, $8::jsonb)
		RETURNING id
	`, ledgerID, jobID, commandID, commandSHA, event.CommandKind,
		event.Kind, event.Authority, payload).Scan(&eventID); err != nil {
		t.Fatal(err)
	}

	badEvent := event
	badEvent.Version = 2
	badCommandID, err := taskstate.NewCommandID(marker, "event-two")
	if err != nil {
		t.Fatal(err)
	}
	badEvent.CommandID = badCommandID
	badPayload, err := json.Marshal(badEvent)
	if err != nil {
		t.Fatal(err)
	}
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `
		INSERT INTO task_events (
			ledger_id, job_id, job_generation, ledger_version, command_id, command_sha256,
			command_kind, event_kind, actor, payload
		) VALUES ($1, $2, 1, 2, $3, $4, $5, $6, $7, $8::jsonb)
	`, ledgerID, jobID, badCommandID, strings.Repeat("b", 64), event.CommandKind,
		event.Kind, event.Authority, badPayload)
	expectTaskLedgerDatabaseFailure(t, ctx, tx,
		`UPDATE task_events SET payload='{}'::jsonb WHERE id=$1`, eventID)
	expectTaskLedgerDatabaseFailure(t, ctx, tx,
		`DELETE FROM task_events WHERE id=$1`, eventID)
	var truncateTriggerExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_trigger trigger
			JOIN pg_class relation ON relation.oid = trigger.tgrelid
			WHERE relation.relname = 'task_events'
			  AND trigger.tgname = 'task_events_prevent_truncate'
			  AND NOT trigger.tgisinternal
		)
	`).Scan(&truncateTriggerExists); err != nil {
		t.Fatal(err)
	}
	if !truncateTriggerExists {
		t.Fatal("PostgreSQL task events table has no truncate-prevention trigger")
	}
	expectTaskLedgerDatabaseFailure(t, ctx, tx, `TRUNCATE task_events`)
	expectTaskLedgerDatabaseFailure(t, ctx, tx,
		`UPDATE task_ledgers SET status='closed' WHERE id=$1`, ledgerID)
}

func insertTaskLedgerTestJob(t *testing.T, ctx context.Context, tx pgx.Tx, marker string) (int64, string) {
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
		VALUES ($1, 'assistant', 'pending', jsonb_build_object('telemetry_run_id', $2::text))
		RETURNING id
	`, marker, runID).Scan(&jobID); err != nil {
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
