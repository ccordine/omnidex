package queue

import (
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresAcceptedIntentObjectiveCompletesWithVerifiedJobAuthority(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("accepted-intent-complete-%d", time.Now().UnixNano())
	job, stepID := enqueueAndProjectAcceptedIntent(t, repository, marker)

	var terminal CompleteStepCommand
	for {
		var action string
		if err := pool.QueryRow(ctx, `SELECT action FROM job_steps WHERE id=$1`, stepID).Scan(&action); err != nil {
			t.Fatal(err)
		}
		terminal = CompleteStepCommand{
			OperationID: testLifecycleOperationID(t, "accepted-intent-complete", stepID),
			StepID:      stepID, Output: "verified completion for " + action,
		}
		if err := repository.CompleteStep(ctx, terminal); err != nil {
			t.Fatal(err)
		}
		details, err := repository.CurrentJobDetails(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if details.Job.Status == model.JobStatusCompleted {
			break
		}
		claimed, err := repository.ClaimNextStep(ctx, marker+fmt.Sprintf("-worker-%d", stepID))
		if err != nil {
			t.Fatal(err)
		}
		if claimed == nil || claimed.Job.ID != job.ID {
			t.Fatalf("claimed step=%+v, want job %d", claimed, job.ID)
		}
		stepID = claimed.Step.ID
	}
	if err := repository.CompleteStep(ctx, terminal); err != nil {
		t.Fatalf("exact terminal completion replay failed: %v", err)
	}

	state, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	objective := acceptedIntentObjective(t, state)
	root := taskLedgerMutationNodes(state.Nodes)[initialTaskRootNodeID]
	if state.Status != taskstate.LedgerClosed || objective.Status != taskstate.NodeDone ||
		root.Status != taskstate.NodeDone || objective.CompletedStepID == nil ||
		*objective.CompletedStepID != terminal.StepID || len(objective.VerificationRefs) != 1 {
		t.Fatalf("terminal ledger/objective/root=%q/%+v/%+v", state.Status, objective, root)
	}
	wantRef := stepCompletionRef(job.ID, 1, terminal.StepID, terminal.Output)
	if objective.VerificationRefs[0] != wantRef {
		t.Fatalf("objective proof=%+v want %+v", objective.VerificationRefs[0], wantRef)
	}
}

func TestPostgresAcceptedIntentObjectiveFailsAndCancelsAtomically(t *testing.T) {
	cases := []struct {
		name       string
		jobStatus  string
		ledger     taskstate.LedgerStatus
		nodeStatus taskstate.NodeStatus
		transition func(*Repository, int64, int64, string) error
	}{
		{
			name: "failed", jobStatus: model.JobStatusFailed,
			ledger: taskstate.LedgerFailed, nodeStatus: taskstate.NodeFailed,
			transition: func(repository *Repository, _ int64, stepID int64, marker string) error {
				return repository.FailStep(t.Context(), FailStepCommand{
					OperationID: testLifecycleOperationID(t, marker+"-fail", stepID),
					StepID:      stepID, Error: "explicit accepted intent failure",
				})
			},
		},
		{
			name: "canceled", jobStatus: model.JobStatusCanceled,
			ledger: taskstate.LedgerCanceled, nodeStatus: taskstate.NodeCanceled,
			transition: func(repository *Repository, jobID int64, _ int64, _ string) error {
				_, err := repository.CancelJob(t.Context(), testCancelCommand(
					t, jobID, "accepted-intent-cancel", "explicit accepted intent cancellation",
				))
				return err
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repository, _, ctx := replanTestRepository(t)
			marker := fmt.Sprintf("accepted-intent-%s-%d", testCase.name, time.Now().UnixNano())
			job, stepID := enqueueAndProjectAcceptedIntent(t, repository, marker)
			if err := testCase.transition(repository, job.ID, stepID, marker); err != nil {
				t.Fatal(err)
			}
			state, err := repository.TaskLedger(ctx, job.ID)
			if err != nil {
				t.Fatal(err)
			}
			objective := acceptedIntentObjective(t, state)
			root := taskLedgerMutationNodes(state.Nodes)[initialTaskRootNodeID]
			if state.Status != testCase.ledger || objective.Status != testCase.nodeStatus ||
				root.Status != testCase.nodeStatus {
				t.Fatalf("terminal ledger/objective/root=%q/%q/%q", state.Status, objective.Status, root.Status)
			}
		})
	}
}

func TestPostgresAcceptedIntentObjectiveSurvivesReplanWithoutStepAssignment(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("accepted-intent-replan-%d", time.Now().UnixNano())
	job, intentStepID := enqueueAndProjectAcceptedIntent(t, repository, marker)
	if err := repository.CompleteStep(ctx, CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, marker+"-complete-intent", intentStepID),
		StepID:      intentStepID, Output: "accepted intent persisted",
	}); err != nil {
		t.Fatal(err)
	}
	replanned, err := repository.ReplanJob(ctx, testReplanCommand(
		t, job.ID, marker, "Reconsider the remaining execution while preserving accepted intent.",
	))
	if err != nil {
		t.Fatal(err)
	}
	if replanned.CurrentGeneration != 2 {
		t.Fatalf("replanned generation=%d want 2", replanned.CurrentGeneration)
	}
	state, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	objective := acceptedIntentObjective(t, state)
	if objective.Status != taskstate.NodeActive || objective.AssignedStepID != nil ||
		objective.CreatedStepID == nil || *objective.CreatedStepID != intentStepID {
		t.Fatalf("replanned accepted objective=%+v", objective)
	}
	var projections int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM task_artifact_projections WHERE job_id=$1
	`, job.ID).Scan(&projections); err != nil {
		t.Fatal(err)
	}
	if projections != 1 {
		t.Fatalf("replan produced %d accepted intent projections", projections)
	}
}

func enqueueAndProjectAcceptedIntent(
	t *testing.T,
	repository *Repository,
	marker string,
) (model.Job, int64) {
	t.Helper()
	job, err := repository.EnqueueJob(t.Context(), marker, model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextStep(t.Context(), marker+"-intent-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Job.ID != job.ID || claimed.Step.Action != "v3_intent_parse" {
		t.Fatalf("claimed step=%+v, want job %d intent", claimed, job.ID)
	}
	if err := repository.WriteAcceptedIntentArtifact(
		t.Context(), acceptedIntentTestEnvelope(t, job.ID, claimed.Step.ID),
	); err != nil {
		t.Fatal(err)
	}
	return job, claimed.Step.ID
}
