package worker

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func invokeCompleteClaimedStep(
	ctx context.Context,
	complete stepCompleteFunc,
	claim *model.ClaimedStep,
	output, contextKey, contextValue string,
) error {
	command, err := completeClaimedStepCommand(claim, output, contextKey, contextValue)
	if err != nil {
		return err
	}
	return complete(ctx, command)
}

func completeClaimedStepCommand(
	claim *model.ClaimedStep,
	output, contextKey, contextValue string,
) (queue.CompleteStepCommand, error) {
	id, err := claimedStepLifecycleOperationID(claim, queue.LifecycleCompleteStep)
	if err != nil {
		return queue.CompleteStepCommand{}, err
	}
	return queue.CompleteStepCommand{
		OperationID: id, Authority: claim.Authority, StepID: claim.Step.ID, Output: output,
		ContextKey: contextKey, ContextValue: contextValue,
	}, nil
}

func failClaimedStepCommand(claim *model.ClaimedStep, failure string) (queue.FailStepCommand, error) {
	id, err := claimedStepLifecycleOperationID(claim, queue.LifecycleFailStep)
	if err != nil {
		return queue.FailStepCommand{}, err
	}
	return queue.FailStepCommand{
		OperationID: id, Authority: claim.Authority, StepID: claim.Step.ID, Error: failure,
	}, nil
}

func claimedStepLifecycleOperationID(
	claim *model.ClaimedStep,
	kind queue.LifecycleOperationKind,
) (queue.LifecycleOperationID, error) {
	if claim == nil || claim.Job.ID <= 0 || claim.Step.ID <= 0 || claim.Step.Generation <= 0 ||
		claim.Step.JobID != claim.Job.ID || claim.Step.Generation != claim.Job.CurrentGeneration ||
		claim.Authority.JobID != claim.Job.ID || claim.Authority.Generation != claim.Step.Generation ||
		claim.Authority.StepID != claim.Step.ID || claim.Authority.Attempt <= 0 ||
		claim.Authority.WorkerID == "" {
		return "", fmt.Errorf("claimed step lifecycle identity requires exact current attempt authority")
	}
	return queue.NewLifecycleOperationID(
		"worker-step-v1",
		strconv.FormatInt(claim.Job.ID, 10),
		strconv.FormatInt(claim.Step.Generation, 10),
		strconv.FormatInt(claim.Step.ID, 10),
		strconv.FormatInt(claim.Authority.Attempt, 10),
		claim.Authority.WorkerID,
		string(kind),
	)
}
