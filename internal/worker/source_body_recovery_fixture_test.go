package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func sourceRecoverySumDefect(t *testing.T, body string) *assemblyline.SourceBodyDefect {
	t.Helper()
	const wrong = "left - right"
	start := strings.Index(body, wrong)
	if start < 0 {
		t.Fatalf("source body lacks expected defect: %q", body)
	}
	defect, err := assemblyline.NewSourceBodyDefect(
		body,
		start,
		start+len(wrong),
		"Which expression computes the required sum?",
		fmt.Errorf("subtraction does not satisfy the required sum"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return defect
}

func reclaimEvidenceAttemptForTest(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
	workerID string,
) *model.ClaimedStep {
	t.Helper()
	if claim == nil {
		t.Fatal("source recovery requires an initial claim")
	}
	terminal, err := pool.Exec(ctx, `
		UPDATE job_step_attempts SET status='expired',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID)
	if err != nil || terminal.RowsAffected() != 1 {
		t.Fatalf("expire attempt rows=%d err=%v", terminal.RowsAffected(), err)
	}
	nextAttempt := claim.Authority.Attempt + 1
	inserted, err := pool.Exec(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES ($1,$2,$3,$4,$5,clock_timestamp(),clock_timestamp())
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		nextAttempt, workerID)
	if err != nil || inserted.RowsAffected() != 1 {
		t.Fatalf("insert reclaimed attempt rows=%d err=%v", inserted.RowsAffected(), err)
	}
	advanced, err := pool.Exec(ctx, `
		UPDATE job_steps SET worker_id=$4,current_attempt=$3,updated_at=clock_timestamp()
		WHERE id=$1 AND job_id=$2 AND status='running'
	`, claim.Authority.StepID, claim.Authority.JobID, nextAttempt, workerID)
	if err != nil || advanced.RowsAffected() != 1 {
		t.Fatalf("advance reclaimed step rows=%d err=%v", advanced.RowsAffected(), err)
	}
	recovered := *claim
	recovered.Authority = model.StepAttemptAuthority{
		JobID: claim.Authority.JobID, Generation: claim.Authority.Generation,
		StepID: claim.Authority.StepID, Attempt: nextAttempt, WorkerID: workerID,
	}
	recovered.Step.WorkerID = workerID
	recovered.LeaseDeadline = time.Now().Add(time.Minute)
	return &recovered
}
