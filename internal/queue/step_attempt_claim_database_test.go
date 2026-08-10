package queue

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresClaimCreatesExactFirstStepAttempt(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	job := enqueueWorkingSetTestJob(t, ctx, repository, fmt.Sprintf("attempt-first-%d", time.Now().UnixNano()))
	claim, err := repository.ClaimNextStep(ctx, "worker-first")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v, want job %d", claim, job.ID)
	}
	want := model.StepAttemptAuthority{
		JobID: job.ID, Generation: 1, StepID: claim.Step.ID, Attempt: 1, WorkerID: "worker-first",
	}
	if claim.Authority != want {
		t.Fatalf("authority=%+v, want %+v", claim.Authority, want)
	}
	if remaining := time.Until(claim.LeaseExpiresAt); remaining < 70*time.Second || remaining > 76*time.Second {
		t.Fatalf("lease remaining=%s, want approximately 75s", remaining)
	}
	var status model.StepAttemptStatus
	var current int64
	if err := pool.QueryRow(ctx, `
		SELECT attempts.status,steps.current_attempt
		FROM job_step_attempts attempts
		JOIN job_steps steps ON steps.id=attempts.step_id
		WHERE attempts.job_id=$1 AND attempts.step_id=$2 AND attempts.attempt=1
	`, job.ID, claim.Step.ID).Scan(&status, &current); err != nil {
		t.Fatal(err)
	}
	if status != model.StepAttemptActive || current != 1 {
		t.Fatalf("persisted status=%q current=%d", status, current)
	}
}

func TestPostgresClaimReclaimsOnlyExpiredRunningAttempt(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	job := enqueueWorkingSetTestJob(t, ctx, repository, fmt.Sprintf("attempt-reclaim-%d", time.Now().UnixNano()))
	var stepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps WHERE job_id=$1 ORDER BY sort_index,id LIMIT 1
	`, job.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * StepAttemptLeaseDuration)
	if _, err := pool.Exec(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES ($1,1,$2,1,'worker-expired',$3,$3)
	`, job.ID, stepID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_steps SET status='running',worker_id='worker-expired',current_attempt=1,
			started_at=$2,updated_at=$2 WHERE id=$1
	`, stepID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE jobs SET status='running',updated_at=$2 WHERE id=$1
	`, job.ID, old); err != nil {
		t.Fatal(err)
	}

	claim, err := repository.ClaimNextStep(ctx, "worker-replacement")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Authority.Attempt != 2 || claim.Authority.WorkerID != "worker-replacement" {
		t.Fatalf("replacement claim=%+v", claim)
	}
	var oldStatus model.StepAttemptStatus
	if err := pool.QueryRow(ctx, `
		SELECT status FROM job_step_attempts
		WHERE job_id=$1 AND generation=1 AND step_id=$2 AND attempt=1
	`, job.ID, stepID).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != model.StepAttemptExpired {
		t.Fatalf("old attempt status=%q, want expired", oldStatus)
	}
	if _, err := repository.RenewStepAttempt(ctx, model.StepAttemptAuthority{
		JobID: job.ID, Generation: 1, StepID: stepID, Attempt: 1, WorkerID: "worker-expired",
	}); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("old renewal error=%v, want ErrStaleStepAttempt", err)
	}
}

func TestPostgresRenewCannotReviveExpiredActiveAttempt(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	job := enqueueWorkingSetTestJob(t, ctx, repository, fmt.Sprintf("attempt-renew-expired-%d", time.Now().UnixNano()))
	var stepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps WHERE job_id=$1 ORDER BY sort_index,id LIMIT 1
	`, job.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * StepAttemptLeaseDuration)
	if _, err := pool.Exec(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES ($1,1,$2,1,'worker-expired-renew',$3,$3)
	`, job.ID, stepID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_steps
		SET status='running',worker_id='worker-expired-renew',current_attempt=1,
			started_at=$2,updated_at=$2
		WHERE id=$1
	`, stepID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status='running' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	authority := model.StepAttemptAuthority{
		JobID: job.ID, Generation: 1, StepID: stepID,
		Attempt: 1, WorkerID: "worker-expired-renew",
	}
	var beforeRenewal time.Time
	if err := pool.QueryRow(ctx, `
		SELECT renewed_at FROM job_step_attempts
		WHERE job_id=$1 AND generation=1 AND step_id=$2 AND attempt=1
	`, job.ID, stepID).Scan(&beforeRenewal); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RenewStepAttempt(ctx, authority); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("expired renewal error=%v want ErrStaleStepAttempt", err)
	}
	var renewedAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT renewed_at FROM job_step_attempts
		WHERE job_id=$1 AND generation=1 AND step_id=$2 AND attempt=1
	`, job.ID, stepID).Scan(&renewedAt); err != nil {
		t.Fatal(err)
	}
	if !renewedAt.Equal(beforeRenewal) {
		t.Fatalf("expired attempt renewed_at=%s want unchanged %s", renewedAt, beforeRenewal)
	}
}

func TestPostgresWaitingStepIsNotReclaimable(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	enqueueWorkingSetTestJob(t, ctx, repository, fmt.Sprintf("attempt-wait-%d", time.Now().UnixNano()))
	claim, err := repository.ClaimNextStep(ctx, "worker-wait")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_step_attempts SET status='waiting_input',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID, claim.Authority.Attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_steps SET status='waiting_input' WHERE id=$1
	`, claim.Authority.StepID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE jobs SET status='waiting_input' WHERE id=$1
	`, claim.Authority.JobID); err != nil {
		t.Fatal(err)
	}
	got, err := repository.ClaimNextStep(ctx, "worker-other")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("waiting step was reclaimed: %+v", got)
	}
}

func TestPostgresExpiredAttemptClaimRaceHasOneReplacement(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	job := enqueueWorkingSetTestJob(t, ctx, repository, fmt.Sprintf("attempt-race-%d", time.Now().UnixNano()))
	var stepID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM job_steps WHERE job_id=$1 ORDER BY sort_index,id LIMIT 1`, job.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * StepAttemptLeaseDuration)
	if _, err := pool.Exec(ctx, `
		INSERT INTO job_step_attempts (job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at)
		VALUES ($1,1,$2,1,'worker-expired',$3,$3)
	`, job.ID, stepID, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_steps SET status='running',worker_id='worker-expired',current_attempt=1 WHERE id=$1
	`, stepID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE jobs SET status='running' WHERE id=$1
	`, job.ID); err != nil {
		t.Fatal(err)
	}

	results := make(chan *model.ClaimedStep, 2)
	errs := make(chan error, 2)
	for _, worker := range []string{"worker-race-one", "worker-race-two"} {
		go func(workerID string) {
			claimed, claimErr := repository.ClaimNextStep(context.Background(), workerID)
			results <- claimed
			errs <- claimErr
		}(worker)
	}
	accepted := 0
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if claim := <-results; claim != nil && claim.Job.ID == job.ID {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("replacement claims=%d, want exactly one", accepted)
	}
}
