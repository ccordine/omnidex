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
	job, err := assemblyline.NewGroundedAnswerJob(input)
	if err != nil {
		return assemblyline.GroundedAnswerDecision{}, objectiveStationReceipt{}, err
	}
	return runRepositoryGroundedStationCall(
		ctx, adapter, station.GroundedAnswer, "grounded_answer", job,
		func(value assemblyline.GroundedAnswerDecision) error { return value.ValidateFor(input) },
	)
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
	job, err := assemblyline.NewRepositoryGroundedReviewJob(input)
	if err != nil {
		return assemblyline.RepositoryGroundedReviewDecision{}, objectiveStationReceipt{}, err
	}
	return runRepositoryGroundedStationCall(
		ctx, adapter, station.RepositoryGroundedReview, "repository_grounded_review", job,
		func(value assemblyline.RepositoryGroundedReviewDecision) error { return value.ValidateFor(input) },
	)
}

func (adapter *portableObjectiveRepositoryGroundingStation) Correct(
	ctx context.Context,
	input assemblyline.RepositoryGroundedCorrectionInput,
) (assemblyline.RepositoryGroundedCorrectionDecision, objectiveStationReceipt, error) {
	job, err := assemblyline.NewRepositoryGroundedCorrectionJob(input)
	if err != nil {
		return assemblyline.RepositoryGroundedCorrectionDecision{}, objectiveStationReceipt{}, err
	}
	return runRepositoryGroundedStationCall(
		ctx, adapter, station.RepositoryGroundedCorrection, "repository_grounded_correction", job,
		func(value assemblyline.RepositoryGroundedCorrectionDecision) error { return value.ValidateFor(input) },
	)
}

func runRepositoryGroundedStationCall[T any](
	ctx context.Context,
	adapter *portableObjectiveRepositoryGroundingStation,
	id station.ID,
	subject string,
	job assemblyline.PortableJob,
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
	value, err := runDirectCodingSemanticCall[T](
		workerRuntime, modelName, subject, job, nil, validate,
	)
	return value, objectiveStationReceipt{Calls: calls}, err
}
