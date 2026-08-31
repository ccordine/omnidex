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
	validate func(T) error,
) (T, objectiveStationReceipt, error) {
	var zero T
	if ctx == nil || runtime == nil || runtime.svc == nil || runtime.claim == nil ||
		resolveModel == nil || decode == nil || validate == nil {
		return zero, objectiveStationReceipt{}, fmt.Errorf(
			"objective raw leaf requires exact running step authority",
		)
	}
	if err := ctx.Err(); err != nil {
		return zero, objectiveStationReceipt{}, err
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
