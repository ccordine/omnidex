package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

func validateObjectiveTextTransportBoundary(label, value string) error {
	if strings.Contains(value, llm.MinimalGeneratePrompt) {
		return fmt.Errorf("%s exposed the private provider prompt hint", label)
	}
	return nil
}

type portableObjectiveKindStation struct {
	runtime *nativeRuntimeV3
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

type portableObjectiveRoleplayCanonStation struct {
	runtime *nativeRuntimeV3
}

func (adapter portableObjectiveRoleplayCanonStation) ExtractCanon(
	ctx context.Context,
	input assemblyline.RoleplayCanonExtractionInput,
) (assemblyline.RoleplayCanonExtractionDecision, objectiveStationReceipt, error) {
	model, err := objectiveStationModel(adapter.runtime, station.RoleplayCanonExtraction)
	if err != nil {
		return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{}, err
	}
	job, err := assemblyline.NewRoleplayCanonExtractionJob(input)
	if err != nil {
		return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{}, err
	}
	var resolved assemblyline.RoleplayCanonExtractionDecision
	decision, calls, err := runObjectivePortableCall[assemblyline.RoleplayCanonExtractionDecision](
		ctx, adapter.runtime, model, "roleplay_canon_extraction", job,
		func(value assemblyline.RoleplayCanonExtractionDecision) error {
			var resolveErr error
			resolved, resolveErr = value.ResolveFor(input)
			return resolveErr
		},
	)
	if err == nil {
		decision = resolved
	}
	return decision, objectiveStationReceipt{Calls: calls}, err
}

func (adapter portableObjectiveConversationStation) Respond(
	ctx context.Context,
	input assemblyline.ConversationResponseInput,
	requestedModel string,
) (assemblyline.ConversationResponseDecision, objectiveStationReceipt, error) {
	model := strings.TrimSpace(requestedModel)
	if model == "" {
		var err error
		model, err = objectiveStationModel(adapter.runtime, station.ConversationResponse)
		if err != nil {
			return assemblyline.ConversationResponseDecision{}, objectiveStationReceipt{}, err
		}
	}
	job, err := assemblyline.NewConversationResponseJob(input)
	if err != nil {
		return assemblyline.ConversationResponseDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableCall[assemblyline.ConversationResponseDecision](
		ctx, adapter.runtime, model, "conversation_response", job,
		func(value assemblyline.ConversationResponseDecision) error {
			if err := value.ValidateFor(input); err != nil {
				return err
			}
			return validateObjectiveTextTransportBoundary("conversation response", value.Text)
		},
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
