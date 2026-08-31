package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type portableObjectiveRepositoryGroundingStation struct {
	runtime *nativeRuntimeV3
}

func newPortableObjectiveRepositoryGroundingStation(
	runtime *nativeRuntimeV3,
) (*portableObjectiveRepositoryGroundingStation, error) {
	if runtime == nil {
		return nil, fmt.Errorf("repository grounding stations require runtime authority")
	}
	return &portableObjectiveRepositoryGroundingStation{runtime: runtime}, nil
}

func (adapter *portableObjectiveRepositoryGroundingStation) Answer(
	ctx context.Context,
	input assemblyline.GroundedAnswerInput,
) (assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error) {
	if adapter == nil || adapter.runtime == nil {
		return assemblyline.GroundedAnswerDecision{}, objectiveStationReceipt{}, fmt.Errorf(
			"repository grounding answer station requires runtime authority",
		)
	}
	if err := input.Validate(); err != nil {
		return assemblyline.GroundedAnswerDecision{}, objectiveStationReceipt{}, err
	}
	resolveModel := func() (string, error) {
		return objectiveStationModel(adapter.runtime, station.GroundedAnswer)
	}
	return resolveRepositoryGroundedParagraphQueue(
		ctx,
		input,
		func(
			ctx context.Context,
			leafInput assemblyline.GroundedAnswerParagraphInventoryInput,
		) (assemblyline.GroundedAnswerParagraphInventory, objectiveStationReceipt, error) {
			job, err := assemblyline.NewGroundedAnswerParagraphInventoryJob(leafInput)
			if err != nil {
				return assemblyline.GroundedAnswerParagraphInventory{}, objectiveStationReceipt{}, err
			}
			return runObjectivePortableRawLeafStation(
				ctx, adapter.runtime, "grounded_answer_paragraph_inventory", job,
				station.GroundedAnswer, resolveModel,
				func(raw string) (assemblyline.GroundedAnswerParagraphInventory, error) {
					return assemblyline.DecodeGroundedAnswerParagraphInventory(leafInput, raw)
				},
				func(value assemblyline.GroundedAnswerParagraphInventory) error {
					return value.ValidateFor(leafInput)
				},
			)
		},
		func(
			ctx context.Context,
			leafInput assemblyline.GroundedAnswerParagraphEvidenceRelationInput,
		) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, objectiveStationReceipt, error) {
			job, err := assemblyline.NewGroundedAnswerParagraphEvidenceRelationJob(leafInput)
			if err != nil {
				return assemblyline.GroundedAnswerParagraphEvidenceRelationDecision{}, objectiveStationReceipt{}, err
			}
			return runObjectivePortableRawLeafStation(
				ctx, adapter.runtime, "grounded_answer_paragraph_evidence_relation", job,
				station.GroundedAnswer, resolveModel,
				func(raw string) (assemblyline.GroundedAnswerParagraphEvidenceRelationDecision, error) {
					return assemblyline.DecodeGroundedAnswerParagraphEvidenceRelationDecision(leafInput, raw)
				},
				func(value assemblyline.GroundedAnswerParagraphEvidenceRelationDecision) error {
					return value.ValidateFor(leafInput)
				},
			)
		},
		func(
			ctx context.Context,
			leafInput assemblyline.GroundedAnswerParagraphAuthorizationInput,
		) (assemblyline.GroundedAnswerParagraphAuthorizationDecision, objectiveStationReceipt, error) {
			job, err := assemblyline.NewGroundedAnswerParagraphAuthorizationJob(leafInput)
			if err != nil {
				return assemblyline.GroundedAnswerParagraphAuthorizationDecision{}, objectiveStationReceipt{}, err
			}
			return runObjectivePortableRawLeafStation(
				ctx, adapter.runtime, "grounded_answer_paragraph_authorization", job,
				station.GroundedAnswer, resolveModel,
				func(raw string) (assemblyline.GroundedAnswerParagraphAuthorizationDecision, error) {
					return assemblyline.DecodeGroundedAnswerParagraphAuthorizationDecision(leafInput, raw)
				},
				func(value assemblyline.GroundedAnswerParagraphAuthorizationDecision) error {
					return value.ValidateFor(leafInput)
				},
			)
		},
	)
}
