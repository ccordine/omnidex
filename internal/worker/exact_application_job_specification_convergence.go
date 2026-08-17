package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

type ExactApplicationJobSpecificationConvergence struct {
	SourceOpeningID    int64
	SourceGapOpeningID int64
	PlannerModel       string
	Terminal           string
	WallDuration       time.Duration
	Specification      assemblyline.ApplicationJobSpecification
	Calls              []ExactApplicationJobSpecificationConvergenceCall
}

type ExactApplicationJobSpecificationConvergenceCall struct {
	Number   int
	WorkKind assemblyline.WorkKind
	Model    string
	Replay   ExactStationReplay
}

type exactApplicationJobSpecificationReplay func(
	job assemblyline.PortableJob,
	model string,
	number int,
) (ExactStationReplay, error)

// ConvergeExactApplicationJobSpecification restores the immutable portable
// authority from one historical specification or repair opening, then renders
// and executes the checked-in production review/repair contracts. It performs
// no queue, historical-job, or workspace writes.
func ConvergeExactApplicationJobSpecification(
	ctx context.Context,
	client llm.ExactStationClient,
	point queue.StationCallReplayPoint,
	plannerModel string,
) (ExactApplicationJobSpecificationConvergence, error) {
	boundary, err := loadStationReplayPortableBoundary(point)
	if err != nil {
		return ExactApplicationJobSpecificationConvergence{}, err
	}
	if client == nil {
		return ExactApplicationJobSpecificationConvergence{}, fmt.Errorf(
			"application job specification convergence requires an exact station client",
		)
	}
	return convergeExactApplicationJobSpecificationWithReplay(
		ctx, point.Call.ID, point.Gap.ID, boundary.Job, plannerModel,
		func(job assemblyline.PortableJob, model string, number int) (ExactStationReplay, error) {
			scope := fmt.Sprintf(
				"application-job-specification-convergence:%d:%d:%s",
				point.Call.ID, number, job.ID,
			)
			return replayCurrentPortableStation(ctx, client, point, job, model, scope, nil)
		},
	)
}

func convergeExactApplicationJobSpecificationWithReplay(
	ctx context.Context,
	sourceOpeningID int64,
	sourceGapOpeningID int64,
	job assemblyline.PortableJob,
	plannerModel string,
	replay exactApplicationJobSpecificationReplay,
) (ExactApplicationJobSpecificationConvergence, error) {
	result := ExactApplicationJobSpecificationConvergence{
		SourceOpeningID: sourceOpeningID, SourceGapOpeningID: sourceGapOpeningID,
		PlannerModel: strings.TrimSpace(plannerModel),
		Terminal:     "failed",
	}
	if ctx == nil || replay == nil || sourceOpeningID < 1 || sourceGapOpeningID < 1 {
		return result, fmt.Errorf("application job specification convergence authority is incomplete")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if result.PlannerModel == "" {
		return result, fmt.Errorf("application job specification convergence requires a planner model")
	}
	if err := job.Validate(); err != nil {
		return result, err
	}
	if job.Kind != assemblyline.WorkApplicationJobSpecification {
		return result, fmt.Errorf("application job specification convergence requires work kind %q", assemblyline.WorkApplicationJobSpecification)
	}
	var authority assemblyline.ApplicationJobSpecificationInput
	if err := json.Unmarshal(job.Payload, &authority); err != nil {
		return result, fmt.Errorf("decode application job specification convergence authority: %w", err)
	}

	runtime := typedWorkerRuntime{
		Context: ctx, MaxAttempts: 1,
		Execute: func(current assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			expected, err := exactApplicationJobSpecificationCallModel(
				current, result.PlannerModel,
			)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			if model != expected {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"application job specification convergence model %q differs from %q authority",
					model, expected,
				)
			}
			number := len(result.Calls) + 1
			call, callErr := replay(current, model, number)
			result.Calls = append(result.Calls, ExactApplicationJobSpecificationConvergenceCall{
				Number: number, WorkKind: current.Kind, Model: model, Replay: call,
			})
			var artifactErr *ExactStationReplayArtifactError
			if callErr != nil && !errors.As(callErr, &artifactErr) {
				return assemblyline.PortableResult{}, callErr
			}
			if call.Job.ID != current.ID || call.Job.Kind != current.Kind ||
				string(call.Job.Payload) != string(current.Payload) {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"application job specification replay differs from requested portable job",
				)
			}
			projection, err := assemblyline.NewExactPortableResultProjection(call.Generation.Content)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			return assemblyline.PortableResult{
				JobID: current.ID, Candidate: call.Generation.Content, Projection: &projection,
			}, nil
		},
	}
	var (
		specification assemblyline.ApplicationJobSpecification
		err           error
	)
	specification, err = resolveDirectCodingApplicationJobSpecification(
		runtime, result.PlannerModel,
		"frozen_application_job_specification", authority,
	)
	if err == nil {
		result.Terminal = "accepted"
	}
	if err != nil {
		return result, err
	}
	result.Specification = specification
	return result, nil
}

func exactApplicationJobSpecificationCallModel(
	job assemblyline.PortableJob,
	plannerModel string,
) (string, error) {
	switch job.Kind {
	case assemblyline.WorkApplicationJobSpecification:
		return plannerModel, nil
	case assemblyline.WorkResponseCorrection:
		var correction assemblyline.ResponseCorrectionInput
		if err := json.Unmarshal(job.Payload, &correction); err != nil {
			return "", fmt.Errorf("decode application job specification correction routing: %w", err)
		}
		return plannerModel, nil
	default:
		return "", fmt.Errorf("application job specification convergence dispatched unsupported work kind %q", job.Kind)
	}
}
