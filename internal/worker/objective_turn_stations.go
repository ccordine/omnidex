package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
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
	job, err := assemblyline.NewRoleplayCanonExtractionJob(input)
	if err != nil {
		return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{}, err
	}
	var resolved assemblyline.RoleplayCanonExtractionDecision
	decision, receipt, err := runObjectiveReusablePortableCall[assemblyline.RoleplayCanonExtractionDecision](
		ctx, adapter.runtime, "roleplay_canon_extraction", job,
		station.RoleplayCanonExtraction,
		func() (string, error) { return objectiveRoleplaySemanticModel(adapter.runtime) },
		func(value assemblyline.RoleplayCanonExtractionDecision) error {
			var resolveErr error
			resolved, resolveErr = value.ResolveFor(input)
			return resolveErr
		},
	)
	if err == nil {
		decision = resolved
	}
	return decision, receipt, err
}

func (adapter portableObjectiveConversationStation) Respond(
	ctx context.Context,
	input assemblyline.ConversationResponseInput,
	requestedModel string,
) (assemblyline.ConversationResponseDecision, objectiveStationReceipt, error) {
	job, err := assemblyline.NewConversationResponseJob(input)
	if err != nil {
		return assemblyline.ConversationResponseDecision{}, objectiveStationReceipt{}, err
	}
	resolveModel := func() (string, error) {
		if model := strings.TrimSpace(requestedModel); model != "" {
			return model, nil
		}
		return objectiveStationModel(adapter.runtime, station.ConversationResponse)
	}
	if input.RoleplayIdentity != nil {
		decision, receipt, err := runObjectiveReusablePortableCall[assemblyline.ConversationResponseDecision](
			ctx, adapter.runtime, "conversation_response", job,
			station.ConversationResponse, resolveModel,
			func(value assemblyline.ConversationResponseDecision) error {
				if err := value.ValidateFor(input); err != nil {
					return err
				}
				return validateObjectiveTextTransportBoundary("conversation response", value.Text)
			},
		)
		return decision, receipt, err
	}
	model, err := resolveModel()
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

func objectiveRoleplaySemanticModel(runtime *nativeRuntimeV3) (string, error) {
	if runtime == nil || runtime.svc == nil {
		return "", fmt.Errorf("roleplay semantic stations require runtime authority")
	}
	return runtime.svc.requiredRoleplaySemanticModel(runtime.routing)
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

func runObjectiveReusablePortableCall[T any](
	ctx context.Context,
	runtime *nativeRuntimeV3,
	subject string,
	job assemblyline.PortableJob,
	owner station.ID,
	resolveModel func() (string, error),
	validate func(T) error,
) (T, objectiveStationReceipt, error) {
	var zero T
	if ctx == nil || runtime == nil || runtime.svc == nil || runtime.claim == nil ||
		runtime.svc.reuseRoleplayResult == nil || resolveModel == nil || validate == nil {
		return zero, objectiveStationReceipt{}, fmt.Errorf(
			"reusable roleplay station requires exact running step authority",
		)
	}
	if err := ctx.Err(); err != nil {
		return zero, objectiveStationReceipt{}, err
	}
	reuse, found, err := runtime.svc.reuseRoleplayResult(
		ctx, queue.RoleplayPortableResultReuseRequest{
			Authority: runtime.claim.Authority, Job: job, Station: owner,
		},
	)
	if err != nil {
		return zero, objectiveStationReceipt{}, fmt.Errorf(
			"reuse accepted %s leaf: %w", subject, err,
		)
	}
	if found {
		if err := reuse.Result.ValidateFor(job); err != nil {
			return zero, objectiveStationReceipt{}, fmt.Errorf(
				"validate reused %s result: %w", subject, err,
			)
		}
		value, err := decodeDirectCodingSemanticJSON[T](reuse.Result.Candidate)
		if err == nil {
			err = validate(value)
		}
		if err != nil {
			return zero, objectiveStationReceipt{}, fmt.Errorf(
				"validate reused %s leaf: %w", subject, err,
			)
		}
		return value, objectiveStationReceipt{Reused: true}, nil
	}
	model, err := resolveModel()
	if err != nil {
		return zero, objectiveStationReceipt{}, err
	}
	value, calls, err := runObjectivePortableCall[T](
		ctx, runtime, model, subject, job, validate,
	)
	return value, objectiveStationReceipt{Calls: calls}, err
}
