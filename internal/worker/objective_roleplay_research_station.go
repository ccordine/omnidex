package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

// portableObjectiveRoleplayGroundedStation resolves the only semantic gap in a
// research turn: how the active fictional character should state the bounded,
// code-acquired real-world evidence. An invalid response fails the turn; this
// path never creates a correction or review call.
type portableObjectiveRoleplayGroundedStation struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveRoleplayGroundedStation) RespondGrounded(
	ctx context.Context,
	input assemblyline.RoleplayGroundedResponseInput,
) (assemblyline.RoleplayGroundedResponseDecision, objectiveStationReceipt, error) {
	modelName, err := objectiveStationModel(adapter.runtime, station.ConversationResponse)
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewRoleplayGroundedResponseJob(input)
	if err != nil {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectiveSinglePortableCall(
		ctx, adapter.runtime, modelName, "roleplay_grounded_response", job,
		func(raw string) (assemblyline.RoleplayGroundedResponseDecision, error) {
			return assemblyline.DecodeRoleplayGroundedResponseDecision(input, raw)
		},
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}

func runObjectiveSinglePortableCall[T any](
	ctx context.Context,
	runtime *nativeRuntimeV3,
	modelName, subject string,
	job assemblyline.PortableJob,
	decode func(string) (T, error),
) (T, int, error) {
	var zero T
	if ctx == nil || runtime == nil || runtime.svc == nil || runtime.claim == nil || decode == nil {
		return zero, 0, fmt.Errorf("objective single-call station requires exact running step authority")
	}
	if err := ctx.Err(); err != nil {
		return zero, 0, err
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return zero, 0, fmt.Errorf("objective station %s model is not configured", subject)
	}
	workerRuntime := portableWorkerRuntimeWithContext(runtime, "objective", ctx)
	if workerRuntime.Execute == nil {
		return zero, 0, fmt.Errorf("objective station %s execution is unavailable", subject)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, 0, err
	}
	emitTypedWorker(workerRuntime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1, PromptBytes: len(prompt),
	})
	result, err := workerRuntime.Execute(job, modelName)
	if err != nil {
		emitTypedWorker(workerRuntime, typedWorkerEvent{
			State: typedWorkerFailed, Kind: typedWorkerSemantic, Subject: subject,
			Model: modelName, Attempt: 1, MaxAttempts: 1, Detail: trimForBudget(err.Error(), 1200),
		})
		return zero, 1, fmt.Errorf("objective station %s inference: %w", subject, err)
	}
	validationErr := result.ValidateFor(job)
	var value T
	if validationErr == nil {
		value, validationErr = decode(strings.TrimSpace(result.Candidate))
	}
	validationErr = finalizeTypedWorkerResult(workerRuntime, job, result, validationErr)
	if validationErr != nil {
		emitTypedWorker(workerRuntime, typedWorkerEvent{
			State: typedWorkerFailed, Kind: typedWorkerSemantic, Subject: subject,
			Model: modelName, Attempt: 1, MaxAttempts: 1,
			Detail: trimForBudget(validationErr.Error(), 1200),
		})
		return zero, 1, fmt.Errorf("objective station %s response failed: %w", subject, validationErr)
	}
	emitTypedWorker(workerRuntime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
	})
	return value, 1, nil
}
