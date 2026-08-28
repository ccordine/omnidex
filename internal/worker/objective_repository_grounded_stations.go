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
	guard   *repositoryGroundingModelIdentityGuard
}

func newPortableObjectiveRepositoryGroundingStation(
	runtime *nativeRuntimeV3,
) (*portableObjectiveRepositoryGroundingStation, error) {
	if runtime == nil {
		return nil, fmt.Errorf("repository grounding stations require runtime authority")
	}
	return &portableObjectiveRepositoryGroundingStation{
		runtime: runtime, guard: &repositoryGroundingModelIdentityGuard{},
	}, nil
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
		ExactRequirement: input.ExactRequirement,
		Context:          assemblyline.CloneObjectiveContext(input.Context),
		Evidence:         append([]assemblyline.GroundedEvidenceCapsule(nil), input.Evidence...),
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
			ExactRequirement: input.ExactRequirement,
			Context:          assemblyline.CloneObjectiveContext(input.Context),
			AnswerText:       text.Text, Evidence: evidence,
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

func (adapter *portableObjectiveRepositoryGroundingStation) ValidateRepositoryGrounding() error {
	if adapter == nil || adapter.runtime == nil {
		return fmt.Errorf("repository grounding preflight requires runtime authority")
	}
	return requireIndependentRepositoryReviewRoutes(adapter.runtime.routing)
}

func (adapter *portableObjectiveRepositoryGroundingStation) Review(
	ctx context.Context,
	input assemblyline.RepositoryGroundedReviewInput,
) (assemblyline.RepositoryGroundedReviewDecision, objectiveStationReceipt, error) {
	job, err := assemblyline.NewRepositoryGroundedIssueDetailJob(input)
	if err != nil {
		return assemblyline.RepositoryGroundedReviewDecision{}, objectiveStationReceipt{}, err
	}
	detail, receipt, err := runRepositoryGroundedLeafCall(
		ctx, adapter, station.RepositoryGroundedReview,
		"repository_grounded_issue_detail", job,
		func(raw string) (string, error) {
			return assemblyline.DecodeRepositoryGroundedIssueDetailLeaf(input, raw)
		},
		func(string) error { return nil },
	)
	if err != nil {
		return assemblyline.RepositoryGroundedReviewDecision{}, receipt, err
	}
	if detail == "" {
		decision, err := assemblyline.AssembleRepositoryGroundedReviewDecision(
			input, "", "",
		)
		return decision, receipt, err
	}

	kindInput := assemblyline.RepositoryGroundedIssueKindLeafInput{
		Review: input,
		Detail: detail,
	}
	kindJob, err := assemblyline.NewRepositoryGroundedIssueKindJob(kindInput)
	if err != nil {
		return assemblyline.RepositoryGroundedReviewDecision{}, receipt, err
	}
	kind, kindReceipt, err := runRepositoryGroundedLeafCall(
		ctx, adapter, station.RepositoryGroundedReview,
		"repository_grounded_issue_kind", kindJob,
		func(raw string) (assemblyline.RepositoryGroundedIssueKind, error) {
			return assemblyline.DecodeRepositoryGroundedIssueKindLeaf(kindInput, raw)
		},
		func(assemblyline.RepositoryGroundedIssueKind) error { return nil },
	)
	receipt.Calls += kindReceipt.Calls
	if err != nil {
		return assemblyline.RepositoryGroundedReviewDecision{}, receipt, err
	}
	decision, err := assemblyline.AssembleRepositoryGroundedReviewDecision(
		input, detail, kind,
	)
	return decision, receipt, err
}

func (adapter *portableObjectiveRepositoryGroundingStation) Correct(
	ctx context.Context,
	input assemblyline.RepositoryGroundedCorrectionInput,
) (assemblyline.RepositoryGroundedCorrectionDecision, objectiveStationReceipt, error) {
	job, err := assemblyline.NewRepositoryGroundedCorrectionJob(input)
	if err != nil {
		return assemblyline.RepositoryGroundedCorrectionDecision{}, objectiveStationReceipt{}, err
	}
	return runRepositoryGroundedLeafCall(
		ctx, adapter, station.RepositoryGroundedCorrection, "repository_grounded_correction", job,
		func(raw string) (assemblyline.RepositoryGroundedCorrectionDecision, error) {
			return assemblyline.DecodeRepositoryGroundedCorrectionDecision(input, raw)
		},
		func(value assemblyline.RepositoryGroundedCorrectionDecision) error { return value.ValidateFor(input) },
	)
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
	if adapter == nil || adapter.runtime == nil || adapter.guard == nil {
		return zero, objectiveStationReceipt{}, fmt.Errorf("repository grounding station %q is unavailable", id)
	}
	modelName, err := objectiveStationModel(adapter.runtime, id)
	if err != nil {
		return zero, objectiveStationReceipt{}, err
	}
	if ctx == nil || strings.TrimSpace(modelName) == "" {
		return zero, objectiveStationReceipt{}, fmt.Errorf("repository grounding station %q requires context and model routing", id)
	}
	workerRuntime := portableWorkerRuntimeWithIdentityGuard(
		adapter.runtime, "objective", ctx, adapter.guard.validate,
	)
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
