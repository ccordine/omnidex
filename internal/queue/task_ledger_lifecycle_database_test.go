package queue

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresTerminalJobAndTaskLedgerCommitTogether(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	cases := []struct {
		name       string
		jobStatus  string
		ledger     taskstate.LedgerStatus
		root       taskstate.NodeStatus
		transition func(int64, int64) error
	}{
		{
			name: "completed", jobStatus: model.JobStatusCompleted, ledger: taskstate.LedgerClosed, root: taskstate.NodeDone,
			transition: func(_ int64, stepID int64) error {
				return repository.CompleteStep(ctx, CompleteStepCommand{
					OperationID: testLifecycleOperationID(t, "terminal-complete", stepID),
					StepID:      stepID, Output: "complete",
				})
			},
		},
		{
			name: "failed", jobStatus: model.JobStatusFailed, ledger: taskstate.LedgerFailed, root: taskstate.NodeFailed,
			transition: func(_ int64, stepID int64) error {
				return repository.FailStep(ctx, FailStepCommand{
					OperationID: testLifecycleOperationID(t, "terminal-fail", stepID),
					StepID:      stepID, Error: "explicit test failure",
				})
			},
		},
		{
			name: "canceled", jobStatus: model.JobStatusCanceled, ledger: taskstate.LedgerCanceled, root: taskstate.NodeCanceled,
			transition: func(jobID int64, _ int64) error {
				_, err := repository.CancelJob(ctx, testCancelCommand(
					t, jobID, "ledger-lifecycle-cancel", "explicit test cancellation",
				))
				return err
			},
		},
		{
			name: "feedback completed", jobStatus: model.JobStatusCompleted, ledger: taskstate.LedgerClosed, root: taskstate.NodeDone,
			transition: func(jobID int64, stepID int64) error {
				if err := repository.PauseStepForInput(ctx, stepID, "waiting", "Continue?", nil); err != nil {
					return err
				}
				_, err := repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{
					OperationID: testLifecycleOperationID(t, "terminal-feedback", jobID),
					JobID:       jobID, Feedback: "Continue.",
				})
				return err
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			marker := fmt.Sprintf("task-ledger-lifecycle-%s-%d", testCase.name, time.Now().UnixNano())
			job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			stepID := taskGenerationStepID(t, ctx, pool, job.ID, 1)
			if testCase.jobStatus != model.JobStatusCanceled {
				claimed, err := repository.ClaimNextStep(ctx, marker+"-worker")
				if err != nil {
					t.Fatal(err)
				}
				if claimed == nil || claimed.Job.ID != job.ID || claimed.Step.ID != stepID {
					t.Fatalf("claimed step=%+v, want job %d step %d", claimed, job.ID, stepID)
				}
			}
			if err := testCase.transition(job.ID, stepID); err != nil {
				t.Fatal(err)
			}
			assertTerminalJobLedger(t, repository, job.ID, testCase.jobStatus, testCase.ledger, testCase.root, 1)
		})
	}
}

func TestPostgresSuccessfulJobCompletionRollsBackWhenTaskLedgerIsIncomplete(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("task-ledger-incomplete-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	commandID, err := taskstate.NewCommandID(marker, "unfinished-task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 1, taskstate.AddNodeCommand{
		CommandID: commandID, ExpectedVersion: initialTaskLedgerVersion, Actor: taskstate.AuthorityCode,
		ID: "unfinished-task", Kind: taskstate.NodeTask, Title: "Unfinished task", Priority: 1,
		AcceptanceCriteria: []string{}, Metadata: taskstate.EmptyJSONObject(),
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Job.ID != job.ID {
		t.Fatalf("claimed step=%+v, want job %d", claimed, job.ID)
	}
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "incomplete-rollback", claimed.Step.ID),
		StepID:      claimed.Step.ID, Output: "must roll back",
	}); !errors.Is(err, taskstate.ErrInvalidState) {
		t.Fatalf("incomplete-ledger completion error=%v", err)
	}

	var jobStatus, stepStatus, ledgerStatus string
	var ledgerVersion, eventCount, operationCount int64
	if err := pool.QueryRow(ctx, `
		SELECT jobs.status, steps.status, ledgers.status, ledgers.version,
		       (SELECT COUNT(*) FROM task_events WHERE job_id=jobs.id),
		       (SELECT COUNT(*) FROM job_lifecycle_operations WHERE job_id=jobs.id)
		FROM jobs
		JOIN job_steps AS steps ON steps.job_id=jobs.id AND steps.id=$2
		JOIN task_ledgers AS ledgers ON ledgers.job_id=jobs.id
		WHERE jobs.id=$1
	`, job.ID, claimed.Step.ID).Scan(
		&jobStatus, &stepStatus, &ledgerStatus, &ledgerVersion, &eventCount, &operationCount,
	); err != nil {
		t.Fatal(err)
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning ||
		ledgerStatus != string(taskstate.LedgerActive) ||
		ledgerVersion != int64(initialTaskLedgerVersion+2) || eventCount != int64(initialTaskLedgerVersion+2) ||
		operationCount != 0 {
		t.Fatalf(
			"rollback state job=%q step=%q ledger=%q version=%d events=%d operations=%d",
			jobStatus, stepStatus, ledgerStatus, ledgerVersion, eventCount, operationCount,
		)
	}
}

func assertTerminalJobLedger(
	t *testing.T,
	repository *Repository,
	jobID int64,
	wantJobStatus string,
	wantLedgerStatus taskstate.LedgerStatus,
	wantRootStatus taskstate.NodeStatus,
	wantGeneration int64,
) {
	t.Helper()
	details, err := repository.CurrentJobDetails(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.TaskLedger(t.Context(), jobID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.Status != wantJobStatus || ledger.Status != wantLedgerStatus {
		t.Fatalf("terminal state job=%q ledger=%q, want %q/%q", details.Job.Status, ledger.Status, wantJobStatus, wantLedgerStatus)
	}
	root := taskLedgerMutationNodes(ledger.Nodes)[initialTaskRootNodeID]
	if root.Status != wantRootStatus {
		t.Fatalf("terminal root status=%q, want %q", root.Status, wantRootStatus)
	}
	page, err := repository.ListTaskEvents(t.Context(), jobID, 0, maxTaskEventPageSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) < 1 || page[len(page)-1].Event.Kind != taskstate.EventLedgerClosed {
		t.Fatalf("terminal task events=%+v", page)
	}
	terminal := page[len(page)-1]
	var persistedGeneration int64
	if err := repository.pool.QueryRow(t.Context(), `
		SELECT job_generation FROM task_events WHERE id=$1
	`, terminal.ID).Scan(&persistedGeneration); err != nil {
		t.Fatal(err)
	}
	if persistedGeneration != wantGeneration {
		t.Fatalf("terminal event generation=%d, want %d", persistedGeneration, wantGeneration)
	}
}
