package queue

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresStepLifecycleOperationsReplayAndConflict(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)

	t.Run("complete", func(t *testing.T) {
		job, stepID := claimedLifecycleTestJob(t, repository, model.PipelineCoding, "complete")
		command := CompleteStepCommand{
			OperationID: testLifecycleOperationID(t, "complete-replay", stepID),
			Authority:   stepAttemptAuthorityForTest(t, repository, stepID),
			StepID:      stepID, Output: "accepted output", ContextKey: "result", ContextValue: "accepted context",
		}
		if err := repository.CompleteStep(ctx, command); err != nil {
			t.Fatal(err)
		}
		before := lifecycleOperationCounts(t, repository, job.ID)
		if err := repository.CompleteStep(ctx, command); err != nil {
			t.Fatalf("exact complete replay: %v", err)
		}
		if after := lifecycleOperationCounts(t, repository, job.ID); after != before {
			t.Fatalf("complete replay counts=%v want %v", after, before)
		}
		if _, err := pool.Exec(ctx, `
			UPDATE task_nodes SET status=$3, updated_at=NOW()
			WHERE job_id=$1 AND id=$2
		`, job.ID, initialTaskRootNodeID, taskstate.NodeActive); err != nil {
			t.Fatal(err)
		}
		tamperedReplayErr := repository.CompleteStep(ctx, command)
		if _, err := pool.Exec(ctx, `
			UPDATE task_nodes SET status=$3, updated_at=NOW()
			WHERE job_id=$1 AND id=$2
		`, job.ID, initialTaskRootNodeID, taskstate.NodeDone); err != nil {
			t.Fatal(err)
		}
		if !errors.Is(tamperedReplayErr, taskstate.ErrInvalidState) {
			t.Fatalf("terminal-authority replay error=%v", tamperedReplayErr)
		}
		changed := command
		changed.Output = "changed output"
		if err := repository.CompleteStep(ctx, changed); !errors.Is(err, ErrLifecycleOperationConflict) {
			t.Fatalf("changed complete replay error=%v", err)
		}
		newIdentity := command
		newIdentity.OperationID = testLifecycleOperationID(t, "complete-second-identity", stepID)
		if err := repository.CompleteStep(ctx, newIdentity); !errors.Is(err, ErrStaleStepAttempt) {
			t.Fatalf("status-only complete retry error=%v", err)
		}
		if _, err := pool.Exec(ctx, `
			UPDATE job_lifecycle_operations SET result_job_status='failed' WHERE operation_id=$1
		`, command.OperationID); err == nil {
			t.Fatal("immutable lifecycle operation accepted UPDATE")
		}
		if _, err := pool.Exec(ctx, `
			DELETE FROM job_lifecycle_operations WHERE operation_id=$1
		`, command.OperationID); err == nil {
			t.Fatal("immutable lifecycle operation accepted DELETE")
		}
	})

	t.Run("fail", func(t *testing.T) {
		job, stepID := claimedLifecycleTestJob(t, repository, model.PipelineCoding, "fail")
		authority := stepAttemptAuthorityForTest(t, repository, stepID)
		if err := repository.AppendStepOutput(ctx, authority, "partial diagnostic output"); err != nil {
			t.Fatal(err)
		}
		command := FailStepCommand{
			OperationID: testLifecycleOperationID(t, "fail-replay", stepID),
			Authority:   authority,
			StepID:      stepID, Error: "authoritative failure",
		}
		if err := repository.FailStep(ctx, command); err != nil {
			t.Fatal(err)
		}
		before := lifecycleOperationCounts(t, repository, job.ID)
		if err := repository.FailStep(ctx, command); err != nil {
			t.Fatalf("exact fail replay: %v", err)
		}
		if after := lifecycleOperationCounts(t, repository, job.ID); after != before {
			t.Fatalf("fail replay counts=%v want %v", after, before)
		}
		changed := command
		changed.Error = "different failure"
		if err := repository.FailStep(ctx, changed); !errors.Is(err, ErrLifecycleOperationConflict) {
			t.Fatalf("changed fail replay error=%v", err)
		}
		newIdentity := command
		newIdentity.OperationID = testLifecycleOperationID(t, "fail-second-identity", stepID)
		if err := repository.FailStep(ctx, newIdentity); !errors.Is(err, ErrStaleStepAttempt) {
			t.Fatalf("status-only fail retry error=%v", err)
		}
	})

}

func TestPostgresFeedbackAndReplanOperationsReplayExactly(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)

	t.Run("final feedback", func(t *testing.T) {
		job, stepID := claimedLifecycleTestJob(t, repository, model.PipelineCoding, "feedback")
		if err := repository.PauseStepForInput(
			ctx, stepAttemptAuthorityForTest(t, repository, stepID), "waiting", "Continue?", nil,
		); err != nil {
			t.Fatal(err)
		}
		command := SubmitJobFeedbackCommand{
			OperationID: testLifecycleOperationID(t, "feedback-replay", job.ID),
			JobID:       job.ID, Feedback: "Continue with the accepted decision.",
		}
		first, err := repository.SubmitJobFeedback(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		second, err := repository.SubmitJobFeedback(ctx, command)
		if err != nil {
			t.Fatalf("exact feedback replay: %v", err)
		}
		if first.ID != second.ID || first.Status != second.Status || !first.UpdatedAt.Equal(second.UpdatedAt) {
			t.Fatalf("feedback replay result first=%+v second=%+v", first, second)
		}
		changed := command
		changed.Feedback = "Changed feedback."
		if _, err := repository.SubmitJobFeedback(ctx, changed); !errors.Is(err, ErrLifecycleOperationConflict) {
			t.Fatalf("changed feedback replay error=%v", err)
		}
		newIdentity := command
		newIdentity.OperationID = testLifecycleOperationID(t, "feedback-second-identity", job.ID)
		if _, err := repository.SubmitJobFeedback(ctx, newIdentity); !errors.Is(err, ErrStepNotWritable) {
			t.Fatalf("status-only feedback retry error=%v", err)
		}
	})

	t.Run("concurrent replan", func(t *testing.T) {
		marker := fmt.Sprintf("lifecycle-replan-race-%d", time.Now().UnixNano())
		job, err := repository.EnqueueJob(ctx, marker, model.PipelineAssistant, []byte(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		command := testReplanCommand(t, job.ID, "race-replay", "Replace the current remaining generation.")
		results := make(chan model.Job, 2)
		errorsOut := make(chan error, 2)
		var start sync.WaitGroup
		start.Add(1)
		for range 2 {
			go func() {
				start.Wait()
				result, callErr := repository.ReplanJob(ctx, command)
				results <- result
				errorsOut <- callErr
			}()
		}
		start.Done()
		for range 2 {
			if callErr := <-errorsOut; callErr != nil {
				t.Fatal(callErr)
			}
			if result := <-results; result.CurrentGeneration != 2 {
				t.Fatalf("replan replay generation=%d want 2", result.CurrentGeneration)
			}
		}
		if _, err := pool.Exec(ctx, `
			UPDATE task_entries
			SET status='resolved', disposition_reason='tampered replay authority',
			    disposition_by='code', updated_at=NOW()
			WHERE job_id=$1 AND id=$2
		`, job.ID, replanFeedbackEntryID(2)); err != nil {
			t.Fatal(err)
		}
		tamperedReplayErr := func() error {
			_, callErr := repository.ReplanJob(ctx, command)
			return callErr
		}()
		if _, err := pool.Exec(ctx, `
			UPDATE task_entries
			SET status='active', disposition_reason='', disposition_by=NULL, updated_at=NOW()
			WHERE job_id=$1 AND id=$2
		`, job.ID, replanFeedbackEntryID(2)); err != nil {
			t.Fatal(err)
		}
		if !errors.Is(tamperedReplayErr, taskstate.ErrInvalidState) {
			t.Fatalf("replan feedback-authority replay error=%v", tamperedReplayErr)
		}
		var generationRows, operationRows int
		if err := pool.QueryRow(ctx, `
			SELECT (SELECT COUNT(*) FROM job_generations WHERE job_id=$1),
			       (SELECT COUNT(*) FROM job_lifecycle_operations WHERE job_id=$1)
		`, job.ID).Scan(&generationRows, &operationRows); err != nil {
			t.Fatal(err)
		}
		if generationRows != 2 || operationRows != 1 {
			t.Fatalf("replan rows generations=%d operations=%d", generationRows, operationRows)
		}
		changed := command
		changed.Feedback = "Different feedback under the same identity."
		if _, err := repository.ReplanJob(ctx, changed); !errors.Is(err, ErrLifecycleOperationConflict) {
			t.Fatalf("changed replan replay error=%v", err)
		}
	})
}

func claimedLifecycleTestJob(
	t *testing.T,
	repository *Repository,
	pipeline, label string,
) (model.Job, int64) {
	t.Helper()
	marker := fmt.Sprintf("lifecycle-%s-%d", label, time.Now().UnixNano())
	job, err := repository.EnqueueJob(t.Context(), marker, pipeline, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextStep(t.Context(), marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Job.ID != job.ID {
		t.Fatalf("claimed step=%+v want job %d", claimed, job.ID)
	}
	return job, claimed.Step.ID
}

func lifecycleOperationCounts(t *testing.T, repository *Repository, jobID int64) [3]int {
	t.Helper()
	var counts [3]int
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM job_lifecycle_operations WHERE job_id=$1),
		       (SELECT COUNT(*) FROM task_events WHERE job_id=$1),
		       (SELECT COUNT(*) FROM step_contexts c JOIN job_steps s ON s.id=c.step_id WHERE s.job_id=$1)
	`, jobID).Scan(&counts[0], &counts[1], &counts[2]); err != nil {
		t.Fatal(err)
	}
	return counts
}

func stepAttemptAuthorityForTest(
	t *testing.T,
	repository *Repository,
	stepID int64,
) model.StepAttemptAuthority {
	t.Helper()
	var authority model.StepAttemptAuthority
	err := repository.pool.QueryRow(t.Context(), `
		SELECT job_id,generation,id,current_attempt,COALESCE(worker_id,'')
		FROM job_steps WHERE id=$1
	`, stepID).Scan(
		&authority.JobID, &authority.Generation, &authority.StepID,
		&authority.Attempt, &authority.WorkerID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}
