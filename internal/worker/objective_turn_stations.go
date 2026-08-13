package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type portableObjectiveKindStation struct {
	runtime *nativeRuntimeV3
}

type portableObjectiveContextSelectionStation struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveContextSelectionStation) Select(
	ctx context.Context,
	input assemblyline.ConversationContextSelectionInput,
) (assemblyline.ConversationContextSelectionDecision, objectiveStationReceipt, error) {
	model, err := objectiveStationModel(adapter.runtime, station.ConversationContextSelection)
	if err != nil {
		return assemblyline.ConversationContextSelectionDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewConversationContextSelectionJob(input)
	if err != nil {
		return assemblyline.ConversationContextSelectionDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.ConversationContextSelectionDecision](
		ctx, adapter.runtime, model, "conversation_context_selection", job,
		func(value assemblyline.ConversationContextSelectionDecision) error {
			return value.ValidateFor(input)
		},
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}

func (adapter portableObjectiveContextSelectionStation) SelectMemory(
	ctx context.Context,
	input assemblyline.MemoryContextSelectionInput,
) (assemblyline.MemoryContextSelectionDecision, objectiveStationReceipt, error) {
	model, err := objectiveStationModel(adapter.runtime, station.MemoryContextSelection)
	if err != nil {
		return assemblyline.MemoryContextSelectionDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewMemoryContextSelectionJob(input)
	if err != nil {
		return assemblyline.MemoryContextSelectionDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.MemoryContextSelectionDecision](
		ctx, adapter.runtime, model, "memory_context_selection", job,
		func(value assemblyline.MemoryContextSelectionDecision) error {
			return value.ValidateFor(input)
		},
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}

func (adapter portableObjectiveKindStation) Classify(
	ctx context.Context,
	input assemblyline.ConversationObjectiveKindInput,
) (assemblyline.ConversationObjectiveKindDecision, objectiveStationReceipt, error) {
	model, err := objectiveStationModel(adapter.runtime, station.ConversationObjectiveKind)
	if err != nil {
		return assemblyline.ConversationObjectiveKindDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewConversationObjectiveKindJob(input)
	if err != nil {
		return assemblyline.ConversationObjectiveKindDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.ConversationObjectiveKindDecision](ctx, adapter.runtime, model, "conversation_objective_kind", job, func(value assemblyline.ConversationObjectiveKindDecision) error {
		return value.ValidateFor(input)
	})
	return decision, objectiveStationReceipt{Calls: calls}, err
}

type portableObjectiveConversationStation struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveConversationStation) Respond(
	ctx context.Context,
	input assemblyline.ConversationResponseInput,
) (assemblyline.ConversationResponseDecision, objectiveStationReceipt, error) {
	model, err := objectiveStationModel(adapter.runtime, station.ConversationResponse)
	if err != nil {
		return assemblyline.ConversationResponseDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewConversationResponseJob(input)
	if err != nil {
		return assemblyline.ConversationResponseDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.ConversationResponseDecision](
		ctx, adapter.runtime, model, "conversation_response", job,
		func(value assemblyline.ConversationResponseDecision) error { return value.ValidateFor(input) },
	)
	return decision, objectiveStationReceipt{Calls: calls}, err
}

func objectiveStationModel(runtime *nativeRuntimeV3, id station.ID) (string, error) {
	if runtime == nil || runtime.svc == nil {
		return "", fmt.Errorf("objective station %q requires runtime authority", id)
	}
	return runtime.svc.requiredStationModel(runtime.routing, id)
}

func runObjectivePortableCall[T any](
	ctx context.Context,
	runtime *nativeRuntimeV3,
	modelName, subject string,
	job assemblyline.PortableJob,
	validate func(T) error,
) (T, int, error) {
	var zero T
	if ctx == nil || runtime == nil || runtime.svc == nil || runtime.claim == nil {
		return zero, 0, fmt.Errorf("objective station requires exact running step authority")
	}
	if err := ctx.Err(); err != nil {
		return zero, 0, err
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return zero, 0, fmt.Errorf("objective station %s model is not configured", subject)
	}
	workerRuntime := portableWorkerRuntimeWithContext(runtime, "objective", ctx)
	calls := 0
	execute := workerRuntime.Execute
	workerRuntime.Execute = func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
		calls++
		return execute(job, model)
	}
	value, err := runDirectCodingSemanticCall[T](
		workerRuntime, modelName, subject, job, nil, validate,
	)
	return value, calls, err
}
