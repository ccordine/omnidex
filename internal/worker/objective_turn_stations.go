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
	decision, calls, err := runObjectivePortableRawLeafCall(
		ctx, adapter.runtime, model, "conversation_objective_kind", job,
		func(raw string) (assemblyline.ConversationObjectiveKindDecision, error) {
			return assemblyline.DecodeConversationObjectiveKindDecision(input, raw)
		},
		func(value assemblyline.ConversationObjectiveKindDecision) error {
			return value.ValidateFor(input)
		},
	)
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
	resolveModel := func() (string, error) {
		return objectiveRoleplaySemanticModel(adapter.runtime)
	}
	facts := make([]string, 0, assemblyline.MaxRoleplayCanonFactsPerTurn)
	totalCalls, allReused := 0, true
	for {
		leafInput := assemblyline.RoleplayCanonFactLeafInput{
			Source: input.Source, AntecedentUserTurn: input.AntecedentUserTurn,
			Context: input.Context, AcceptedFacts: append([]string{}, facts...),
		}
		coverageJob, err := assemblyline.NewRoleplayCanonFactCoverageJob(leafInput)
		if err != nil {
			return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		coverage, receipt, err := runObjectiveReusablePortableRawLeafCall(
			ctx, adapter.runtime, "roleplay_canon_fact_coverage", coverageJob,
			station.RoleplayCanonExtraction, resolveModel,
			func(raw string) (string, error) {
				return assemblyline.DecodeRoleplayCanonFactCoverageLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		totalCalls += receipt.Calls
		allReused = allReused && receipt.Reused
		if err != nil {
			return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		if coverage == assemblyline.RoleplayNoUncoveredCanonFact {
			decision, err := assemblyline.AssembleRoleplayCanonExtractionDecision(input, facts)
			return decision, objectiveStationReceipt{
				Calls: totalCalls, Reused: allReused,
			}, err
		}
		if len(facts) == assemblyline.MaxRoleplayCanonFactsPerTurn {
			return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, fmt.Errorf(
				"roleplay canon fact coverage remains incomplete at the code-owned %d-item bound",
				assemblyline.MaxRoleplayCanonFactsPerTurn,
			)
		}
		factJob, err := assemblyline.NewRoleplayCanonFactJob(leafInput)
		if err != nil {
			return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		fact, receipt, err := runObjectiveReusablePortableRawLeafCall(
			ctx, adapter.runtime, "roleplay_canon_fact", factJob,
			station.RoleplayCanonExtraction, resolveModel,
			func(raw string) (string, error) {
				return assemblyline.DecodeRoleplayCanonFactLeaf(leafInput, raw)
			},
			func(string) error { return nil },
		)
		totalCalls += receipt.Calls
		allReused = allReused && receipt.Reused
		if err != nil {
			return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		facts = append(facts, fact)
	}
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
		decision, receipt, err := runObjectiveReusablePortableRawLeafCall(
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
		return decision, receipt, err
	}
	model, err := resolveModel()
	if err != nil {
		return assemblyline.ConversationResponseDecision{}, objectiveStationReceipt{}, err
	}
	decision, calls, err := runObjectivePortableRawLeafCall(
		ctx, adapter.runtime, model, "conversation_response", job,
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
