package queue

import (
	"bytes"
	"context"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func requireCancelJobReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record lifecycleOperationRecord,
	command CancelJobCommand,
	current model.Job,
) error {
	if record.JobID != command.JobID || record.Kind != LifecycleCancelJob ||
		record.StepID != nil || record.ResultStepStatus != nil ||
		record.ResultGeneration != record.ObservedGeneration ||
		record.ResultJobStatus != model.JobStatusCanceled || record.ResultJob.Error != command.Reason {
		return lifecycleReplayStateError(record.ID, "canceled job result")
	}
	if !sameCanceledJobAuthority(current, record.ResultJob) || current.Error != command.Reason {
		return lifecycleReplayStateError(record.ID, "canceled job authority")
	}
	return requireLifecycleGenerationExistsTx(
		ctx, tx, current.ID, current.CurrentGeneration,
	)
}

func sameCanceledJobAuthority(current, recorded model.Job) bool {
	if current.ID != recorded.ID || current.Instruction != recorded.Instruction ||
		current.Pipeline != recorded.Pipeline || current.Status != recorded.Status ||
		current.Result != recorded.Result || current.Error != recorded.Error ||
		current.CurrentGeneration != recorded.CurrentGeneration ||
		!current.CreatedAt.Equal(recorded.CreatedAt) || !current.UpdatedAt.Equal(recorded.UpdatedAt) ||
		!bytes.Equal(current.Metadata, recorded.Metadata) {
		return false
	}
	if current.CompletedAt == nil || recorded.CompletedAt == nil {
		return current.CompletedAt == nil && recorded.CompletedAt == nil
	}
	return current.CompletedAt.Equal(*recorded.CompletedAt)
}
