package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTaskLedgerRepositoryRejectsMissingAuthority(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, err := (&Repository{}).TaskLedger(ctx, 0); err == nil || !strings.Contains(err.Error(), "job ID") {
		t.Fatalf("invalid job error=%v", err)
	}
	if _, err := (&Repository{}).ApplyTaskCommand(ctx, 1, 1, nil); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing command repository error=%v", err)
	}
	if _, err := (&Repository{}).ListTaskEvents(ctx, 1, -1, 1); err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("negative event cursor error=%v", err)
	}
	for _, limit := range []int{0, maxTaskEventPageSize + 1} {
		if _, err := (&Repository{}).ListTaskEvents(ctx, 1, 0, limit); err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatalf("event page limit %d error=%v", limit, err)
		}
	}
}

func TestPostgresTaskLedgerCommandsAreAtomicAndIdempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}

	marker := fmt.Sprintf("task-ledger-repository-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker+"-one", model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	otherJob, err := repository.EnqueueJob(ctx, marker+"-two", model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Version != initialTaskLedgerVersion || ledger.Status != taskstate.LedgerActive || ledger.Owner.JobID != job.ID {
		t.Fatalf("enqueued ledger=%+v", ledger)
	}

	var otherStepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps WHERE job_id=$1 ORDER BY sort_index, id LIMIT 1
	`, otherJob.ID).Scan(&otherStepID); err != nil {
		t.Fatal(err)
	}
	badID, err := taskstate.NewCommandID(marker, "cross-job-step")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.ApplyTaskCommand(ctx, job.ID, 1, taskstate.AddNodeCommand{
		CommandID: badID, ExpectedVersion: initialTaskLedgerVersion, Actor: taskstate.AuthorityCode,
		ID: "bad-node", Kind: taskstate.NodeTask, Title: "Bad node", Priority: 1,
		CreatedStepID: &otherStepID, AcceptanceCriteria: []string{}, Metadata: taskstate.EmptyJSONObject(),
	})
	if err == nil {
		t.Fatal("cross-job step binding silently succeeded")
	}
	assertTaskLedgerDatabaseCounts(t, ctx, pool, ledger.ID, int64(initialTaskLedgerVersion), 1, int64(initialTaskLedgerVersion))

	commandID, err := taskstate.NewCommandID(marker, "valid-node")
	if err != nil {
		t.Fatal(err)
	}
	command := taskstate.AddNodeCommand{
		CommandID: commandID, ExpectedVersion: initialTaskLedgerVersion, Actor: taskstate.AuthorityCode,
		ID: "node-one", Kind: taskstate.NodeTask, Title: "Node one", Priority: 10,
		AcceptanceCriteria: []string{}, Metadata: taskstate.EmptyJSONObject(),
	}
	first, err := repository.ApplyTaskCommand(ctx, job.ID, 1, command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.ApplyTaskCommand(ctx, job.ID, 1, command)
	if err != nil {
		t.Fatal(err)
	}
	if taskLedgerTestEventJSON(t, first) != taskLedgerTestEventJSON(t, replayed) || first.Version != initialTaskLedgerVersion+1 {
		t.Fatalf("replayed event=%s first=%s", taskLedgerTestEventJSON(t, replayed), taskLedgerTestEventJSON(t, first))
	}

	changed := command
	changed.Title = "Reused command identity with changed content"
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 1, changed); !errors.Is(err, taskstate.ErrCommandIDConflict) {
		t.Fatalf("reused command identity error=%v", err)
	}
	staleID, err := taskstate.NewCommandID(marker, "stale-node")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.ApplyTaskCommand(ctx, job.ID, 1, taskstate.AddNodeCommand{
		CommandID: staleID, ExpectedVersion: initialTaskLedgerVersion, Actor: taskstate.AuthorityCode,
		ID: "stale-node", Kind: taskstate.NodeTask, Title: "Stale", Priority: 1,
		AcceptanceCriteria: []string{}, Metadata: taskstate.EmptyJSONObject(),
	})
	if !errors.Is(err, taskstate.ErrVersionConflict) {
		t.Fatalf("stale task command error=%v", err)
	}

	loaded, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Version != initialTaskLedgerVersion+1 || loaded.Status != taskstate.LedgerActive {
		t.Fatalf("loaded ledger version=%d status=%q", loaded.Version, loaded.Status)
	}
	nodes := taskLedgerMutationNodes(loaded.Nodes)
	if node, ok := nodes["node-one"]; !ok || node.Title != command.Title {
		t.Fatalf("loaded node=%+v present=%t", node, ok)
	}
	assertTaskLedgerDatabaseCounts(t, ctx, pool, ledger.ID, int64(initialTaskLedgerVersion+1), 2, int64(initialTaskLedgerVersion+1))

	corruptLedger, err := repository.TaskLedger(ctx, otherJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	seedAggregate, err := taskstate.RestoreLedger(corruptLedger)
	if err != nil {
		t.Fatal(err)
	}
	seedID, err := taskstate.NewCommandID(marker, "seed-corrupt-replay")
	if err != nil {
		t.Fatal(err)
	}
	seedCommand := taskstate.AddEntryCommand{
		CommandID: seedID, ExpectedVersion: initialTaskLedgerVersion, Actor: taskstate.AuthorityCode,
		ID: "corrupt-replay-entry", Kind: taskstate.EntryNote,
		Content:  "This event has a deliberately inconsistent command version.",
		Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{},
	}
	corruptEvent, err := seedAggregate.Apply(seedCommand)
	if err != nil {
		t.Fatal(err)
	}
	corruptCommand := seedCommand
	corruptCommand.CommandID, err = taskstate.NewCommandID(marker, "corrupt-replay")
	if err != nil {
		t.Fatal(err)
	}
	corruptCommand.ExpectedVersion = initialTaskLedgerVersion + 5
	corruptDescriptor, err := taskstate.DescribeCommand(corruptCommand)
	if err != nil {
		t.Fatal(err)
	}
	corruptEvent.CommandID = corruptDescriptor.ID
	corruptEvent.CommandSHA256 = corruptDescriptor.SHA256
	corruptPayload, err := json.Marshal(corruptEvent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE task_ledgers SET version=$2, updated_at=NOW() WHERE id=$1
	`, corruptLedger.ID, int64(initialTaskLedgerVersion+1)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO task_events (
			ledger_id, job_id, job_generation, ledger_version, command_id, command_sha256,
			command_kind, event_kind, actor, step_id, payload
		) VALUES ($1, $2, 1, $3, $4, $5, $6, $7, $8, NULL, $9::jsonb)
	`, corruptLedger.ID, otherJob.ID, int64(initialTaskLedgerVersion+1), corruptEvent.CommandID, corruptEvent.CommandSHA256,
		corruptEvent.CommandKind, corruptEvent.Kind, corruptEvent.Authority, corruptPayload); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyTaskCommand(ctx, otherJob.ID, 1, corruptCommand); !errors.Is(err, taskstate.ErrInvalidState) ||
		!strings.Contains(err.Error(), "command descriptor") {
		t.Fatalf("inconsistent exact command replay error=%v", err)
	}

	var missingRunJobID int64
	missingRunTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer missingRunTx.Rollback(context.Background())
	if err := missingRunTx.QueryRow(ctx, `
		INSERT INTO jobs (instruction, pipeline, status, metadata)
		VALUES ($1, 'agent', 'pending', '{}'::jsonb)
		RETURNING id
	`, marker+"-missing-run").Scan(&missingRunJobID); err != nil {
		t.Fatal(err)
	}
	if _, err := missingRunTx.Exec(ctx, `
		INSERT INTO job_generations (job_id, generation, purpose) VALUES ($1, 1, 'initial')
	`, missingRunJobID); err != nil {
		t.Fatal(err)
	}
	if err := missingRunTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.TaskLedger(ctx, missingRunJobID); !errors.Is(err, taskstate.ErrNotFound) {
		t.Fatalf("job without telemetry authority unexpectedly exposed a task ledger: %v", err)
	}
}

func taskLedgerTestEventJSON(t *testing.T, event taskstate.Event) string {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func assertTaskLedgerDatabaseCounts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	ledgerID taskstate.LedgerID,
	wantVersion, wantNodes, wantEvents int64,
) {
	t.Helper()
	var version, nodes, events int64
	if err := pool.QueryRow(ctx, `
		SELECT ledger.version,
		       (SELECT COUNT(*) FROM task_nodes WHERE ledger_id=ledger.id),
		       (SELECT COUNT(*) FROM task_events WHERE ledger_id=ledger.id)
		FROM task_ledgers ledger WHERE ledger.id=$1
	`, ledgerID).Scan(&version, &nodes, &events); err != nil {
		t.Fatal(err)
	}
	if version != wantVersion || nodes != wantNodes || events != wantEvents {
		t.Fatalf("ledger counts version=%d nodes=%d events=%d, want %d/%d/%d",
			version, nodes, events, wantVersion, wantNodes, wantEvents)
	}
}
