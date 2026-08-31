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
	job, err := assemblyline.NewConversationObjectiveKindJob(input)
	if err != nil {
		return assemblyline.ConversationObjectiveKindDecision{}, objectiveStationReceipt{}, err
	}
	return runObjectivePortableRawLeafStation(
		ctx, adapter.runtime, "conversation_objective_kind", job,
		station.ConversationObjectiveKind,
		func() (string, error) {
			return objectiveStationModel(adapter.runtime, station.ConversationObjectiveKind)
		},
		func(raw string) (assemblyline.ConversationObjectiveKindDecision, error) {
			return assemblyline.DecodeConversationObjectiveKindDecision(input, raw)
		},
		func(value assemblyline.ConversationObjectiveKindDecision) error {
			return value.ValidateFor(input)
		},
	)
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
	return resolveRoleplayCanonCandidateQueue(ctx, adapter, input)
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
		if requestedModel != "" {
			return requestedModel, nil
		}
		return objectiveStationModel(adapter.runtime, station.ConversationResponse)
	}
	return runObjectivePortableRawLeafStation(
		ctx, adapter.runtime, "conversation_response", job,
		station.ConversationResponse, resolveModel,
		func(raw string) (assemblyline.ConversationResponseDecision, error) {
			return assemblyline.DecodeConversationResponseDecision(input, raw)
		},
		func(value assemblyline.ConversationResponseDecision) error {
			if err := value.ValidateFor(input); err != nil {
				return err
			}
			return validateObjectiveTextTransportBoundary("conversation response", value.Text)
		},
	)
}

func objectiveStationModel(runtime *nativeRuntimeV3, id station.ID) (string, error) {
	if runtime == nil || runtime.svc == nil {
		return "", fmt.Errorf("objective station %q requires runtime authority", id)
	}
	routing, err := runtime.modelRouting()
	if err != nil {
		return "", err
	}
	return runtime.svc.requiredStationModel(routing, id)
}

func objectiveRoleplaySemanticModel(runtime *nativeRuntimeV3) (string, error) {
	if runtime == nil || runtime.svc == nil {
		return "", fmt.Errorf("roleplay semantic stations require runtime authority")
	}
	routing, err := runtime.modelRouting()
	if err != nil {
		return "", err
	}
	return runtime.svc.requiredRoleplaySemanticModel(routing)
}
