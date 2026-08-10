package queue

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openWorkingSetDatabase(t *testing.T) (context.Context, *Repository, *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	return ctx, repository, pool
}

func enqueueWorkingSetTestJob(t *testing.T, ctx context.Context, repository *Repository, marker string) model.Job {
	t.Helper()
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func claimWorkingSetTestJob(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	job model.Job,
) model.StepAttemptAuthority {
	t.Helper()
	claim, err := repository.ClaimNextStep(ctx, "working-set-"+strings.TrimPrefix(job.Instruction, "working-set-"))
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claimed step=%+v want job %d", claim, job.ID)
	}
	return claim.Authority
}

func workingSetDatabaseCommandID(t *testing.T, marker, label string) workingset.CommandID {
	t.Helper()
	id, err := workingset.NewCommandID(marker, label)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func workingSetDatabaseRequest(id string, scope workingset.Scope) workingset.AcquireRequest {
	return workingset.AcquireRequest{
		ID: workingset.ItemID(id),
		Ref: taskstate.Ref{
			URI: "repo://snapshot/symbol/" + id, Version: "snapshot-1",
			Hash: strings.Repeat("a", 64), Relation: taskstate.RefEvidence,
		},
		Role: workingset.RoleRepositoryEvidence, Retention: workingset.Retention(scope.Kind),
		Scope: scope, Priority: 80, ByteCost: 8,
		Acquisition: workingset.Acquisition{
			Provider: workingset.ProviderRepository, OperationID: "query-" + id,
			Reason: "Required by the current repository investigation.",
		},
	}
}

func assertWorkingSetEventPages(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	jobID, generation int64,
	want []workingset.Event,
) {
	t.Helper()
	var cursor int64
	for index := range want {
		page, err := repository.ListWorkingSetEvents(ctx, jobID, generation, cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) != 1 || page[0].ID <= cursor || !reflect.DeepEqual(page[0].Event, want[index]) {
			t.Fatalf("event page %d=%+v, want %+v after %d", index, page, want[index], cursor)
		}
		cursor = page[0].ID
	}
	page, err := repository.ListWorkingSetEvents(ctx, jobID, generation, cursor, 1)
	if err != nil || len(page) != 0 {
		t.Fatalf("final event page=%+v error=%v", page, err)
	}
}

func assertWorkingSetEventsImmutable(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	setID workingset.SetID,
) {
	t.Helper()
	for _, statement := range []string{
		`UPDATE working_set_events SET actor='code' WHERE working_set_id=$1`,
		`DELETE FROM working_set_events WHERE working_set_id=$1`,
	} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, statement, setID); err == nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("immutable working-set event accepted %q", statement)
		}
		_ = tx.Rollback(ctx)
	}
}

func advanceWorkingSetTestGeneration(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID int64,
) {
	t.Helper()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var lockedID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM jobs WHERE id=$1 FOR UPDATE`, jobID).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}
	advanceWorkingSetGenerationTx(t, ctx, tx, jobID)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func advanceWorkingSetGenerationTx(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	jobID int64,
) {
	t.Helper()
	feedback := "Create a fresh generation for the working-set authority test."
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_generations (
			job_id, generation, purpose, predecessor_generation,
			boundary_action, feedback, feedback_sha256
		) VALUES ($1,2,'replan',1,'v3_planning',$2,encode(digest($2,'sha256'),'hex'))
	`, jobID, feedback); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET current_generation=2 WHERE id=$1`, jobID); err != nil {
		t.Fatal(err)
	}
}
