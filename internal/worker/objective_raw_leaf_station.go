package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func runObjectivePortableRawLeafStation[T any](
	ctx context.Context,
	runtime *nativeRuntimeV3,
	subject string,
	job assemblyline.PortableJob,
	_ station.ID,
	resolveModel func() (string, error),
	decode objectiveRawLeafDecoder[T],
) (T, int, error) {
	var zero T
	if ctx == nil || decode == nil {
		return zero, 0, fmt.Errorf(
			"objective raw leaf requires exact running step authority",
		)
	}
	if err := ctx.Err(); err != nil {
		return zero, 0, err
	}
	deterministic, resolved, err := assemblyline.ResolvePortableJobWithoutInference(job)
	if err != nil {
		return zero, 0, err
	}
	if resolved {
		value, err := decode(deterministic.Candidate)
		return value, 0, err
	}
	if runtime == nil || runtime.svc == nil || runtime.claim == nil || resolveModel == nil {
		return zero, 0, fmt.Errorf(
			"objective raw leaf requires exact running step authority",
		)
	}
	model, err := resolveModel()
	if err != nil {
		return zero, 0, err
	}
	value, calls, err := runObjectivePortableRawLeafCall(
		ctx, runtime, model, subject, job, decode,
	)
	return value, calls, err
}
