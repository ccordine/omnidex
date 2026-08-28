package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/station"
)

func runObjectiveReusablePortableRawLeafCall[T any](
	ctx context.Context,
	runtime *nativeRuntimeV3,
	subject string,
	job assemblyline.PortableJob,
	owner station.ID,
	resolveModel func() (string, error),
	decode objectiveRawLeafDecoder[T],
	validate func(T) error,
) (T, objectiveStationReceipt, error) {
	var zero T
	if ctx == nil || runtime == nil || runtime.svc == nil || runtime.claim == nil ||
		runtime.svc.reuseRoleplayResult == nil || resolveModel == nil ||
		decode == nil || validate == nil {
		return zero, objectiveStationReceipt{}, fmt.Errorf(
			"reusable objective raw leaf requires exact running step authority",
		)
	}
	if err := ctx.Err(); err != nil {
		return zero, objectiveStationReceipt{}, err
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return zero, objectiveStationReceipt{}, err
	}
	pathProvenance := portableWorkerRuntimeWithContext(
		runtime, "objective", ctx,
	).PathProvenance
	if err := validateDirectCodingSemanticPrompt(
		prompt, nil, pathProvenance,
	); err != nil {
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
		value, err := decode(reuse.Result.Candidate)
		if err == nil {
			err = validateObjectiveRawLeafPathBoundary(value, pathProvenance)
		}
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
	value, calls, err := runObjectivePortableRawLeafCall(
		ctx, runtime, model, subject, job, decode, validate,
	)
	return value, objectiveStationReceipt{Calls: calls}, err
}
