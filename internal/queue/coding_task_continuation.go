package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// Only an actual owner reply crosses back into mutable planning. The planner's
// proposal cannot mutate or approve the frozen execution ledger by itself.
func replanIncompleteCodingTaskTx(ctx context.Context, tx pgx.Tx, owner SubmitJobFeedbackCommand, job model.Job, response codingagent.Entry) (model.Job, error) {
	if response.Role != codingagent.Plan || response.Phase == "task_incomplete" && (response.Continuation == nil || response.Continuation.Task.ID != response.TaskID) {
		return model.Job{}, fmt.Errorf("incomplete task lost its code-owned continuation receipt")
	}
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT current_revision FROM coding_ledgers WHERE job_id=$1),0)`, job.ID).Scan(&revision); err != nil {
		return model.Job{}, err
	}
	command := ReplanJobCommand{OperationID: model.LifecycleOperationID(fmt.Sprintf("lifecycle_operation_continuation_%d_%d_%d", job.ID, job.CurrentGeneration, response.ID)),
		JobID: job.ID, Feedback: owner.Feedback, ObservedLedgerRevision: revision, WorkspaceRoot: owner.WorkspaceRoot, WorkspaceIdentity: owner.WorkspaceIdentity}
	command, err := normalizeReplanJobCommand(command)
	if err != nil {
		return model.Job{}, err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleReplanJob, command)
	if err != nil {
		return model.Job{}, err
	}
	result, err := replanJobTx(ctx, tx, command, descriptor)
	if err != nil {
		return model.Job{}, err
	}
	return result.Job, nil
}
