package worker

import (
	"context"
	"fmt"
	"strings"

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
	textInput := assemblyline.GroundedAnswerTextInput{
		ExactRequirement:   input.ExactRequirement,
		Context:            assemblyline.CloneObjectiveContext(input.Context),
		Evidence:           append([]assemblyline.GroundedEvidenceCapsule(nil), input.Evidence...),
		KnownArtifactPaths: append([]string(nil), input.KnownArtifactPaths...),
	}
	job, err := assemblyline.NewGroundedAnswerTextJob(textInput)
	if err != nil {
		return assemblyline.GroundedAnswerDecision{}, objectiveStationReceipt{}, err
	}
	text, receipt, err := runRepositoryGroundedLeafCall(
		ctx, adapter, station.GroundedAnswer, "grounded_answer_text", job,
		func(raw string) (assemblyline.GroundedAnswerTextDecision, error) {
			return assemblyline.DecodeGroundedAnswerTextDecision(textInput, raw)
		},
		func(value assemblyline.GroundedAnswerTextDecision) error {
			return value.ValidateFor(textInput)
		},
	)
	if err != nil {
		return assemblyline.GroundedAnswerDecision{}, receipt, err
	}
	total := receipt.Calls
	evidenceIDs := make([]string, 0, len(input.Evidence))
	for _, evidence := range input.Evidence {
		relationInput := assemblyline.GroundedAnswerEvidenceRelationInput{
			ExactRequirement:   input.ExactRequirement,
			Context:            assemblyline.CloneObjectiveContext(input.Context),
			AnswerText:         text.Text,
			Evidence:           evidence,
			KnownArtifactPaths: append([]string(nil), input.KnownArtifactPaths...),
		}
		relationJob, err := assemblyline.NewGroundedAnswerEvidenceRelationJob(relationInput)
		if err != nil {
			return assemblyline.GroundedAnswerDecision{}, objectiveStationReceipt{Calls: total}, err
		}
		relation, relationReceipt, err := runRepositoryGroundedLeafCall(
			ctx, adapter, station.GroundedAnswer, "grounded_answer_evidence_relation",
			relationJob,
			func(raw string) (assemblyline.GroundedAnswerEvidenceRelationDecision, error) {
				return assemblyline.DecodeGroundedAnswerEvidenceRelationDecision(relationInput, raw)
			},
			func(value assemblyline.GroundedAnswerEvidenceRelationDecision) error {
				return value.ValidateFor(relationInput)
			},
		)
		total += relationReceipt.Calls
		if err != nil {
			return assemblyline.GroundedAnswerDecision{}, objectiveStationReceipt{Calls: total}, err
		}
		if relation.Relation == assemblyline.GroundedEvidenceSupportsAnswer {
			evidenceIDs = append(evidenceIDs, evidence.ID)
		}
	}
	decision := assemblyline.GroundedAnswerDecision{
		Schema:        assemblyline.GroundedAnswerSchemaV1,
		RequirementID: input.RequirementID,
		Text:          text.Text, EvidenceIDs: evidenceIDs,
	}
	if err := decision.ValidateFor(input); err != nil {
		return assemblyline.GroundedAnswerDecision{}, objectiveStationReceipt{Calls: total}, err
	}
	return decision, objectiveStationReceipt{Calls: total}, nil
}

func runRepositoryGroundedLeafCall[T any](
	ctx context.Context,
	adapter *portableObjectiveRepositoryGroundingStation,
	id station.ID,
	subject string,
	job assemblyline.PortableJob,
	decode objectiveRawLeafDecoder[T],
	validate func(T) error,
) (T, objectiveStationReceipt, error) {
	var zero T
	if adapter == nil || adapter.runtime == nil {
		return zero, objectiveStationReceipt{}, fmt.Errorf("repository grounding station %q is unavailable", id)
	}
	modelName, err := objectiveStationModel(adapter.runtime, id)
	if err != nil {
		return zero, objectiveStationReceipt{}, err
	}
	if ctx == nil || strings.TrimSpace(modelName) == "" {
		return zero, objectiveStationReceipt{}, fmt.Errorf("repository grounding station %q requires context and model routing", id)
	}
	workerRuntime := portableWorkerRuntimeWithContext(adapter.runtime, "objective", ctx)
	calls := 0
	execute := workerRuntime.Execute
	workerRuntime.Execute = func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
		calls++
		return execute(job, model)
	}
	value, err := runObjectiveRawLeafWorkerCall(
		workerRuntime, modelName, subject, job, decode, validate,
	)
	return value, objectiveStationReceipt{Calls: calls}, err
}
