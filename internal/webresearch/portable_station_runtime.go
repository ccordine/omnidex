package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (stations *PortableStations) run(
	ctx context.Context,
	job assemblyline.PortableJob,
) (assemblyline.PortableResult, error) {
	if stations == nil || stations.runtime.Execute == nil || stations.runtime.Finalize == nil {
		return assemblyline.PortableResult{}, fmt.Errorf("portable web stations are uninitialized")
	}
	if ctx == nil {
		return assemblyline.PortableResult{}, fmt.Errorf("portable web station context is nil")
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, err
	}
	result, err := stations.runtime.Execute(ctx, job)
	if err != nil {
		return assemblyline.PortableResult{}, fmt.Errorf("execute portable web station: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, combinePortableStationErrors(
			err, stations.finalize(ctx, job, result, err),
		)
	}
	if err := result.ValidateFor(job); err != nil {
		validationErr := fmt.Errorf("validate portable web station result: %w", err)
		return assemblyline.PortableResult{}, combinePortableStationErrors(
			validationErr, stations.finalize(ctx, job, result, validationErr),
		)
	}
	return result, nil
}

func (stations *PortableStations) finalize(
	ctx context.Context,
	job assemblyline.PortableJob,
	result assemblyline.PortableResult,
	validationErr error,
) error {
	return stations.runtime.Finalize(ctx, job, result, validationErr)
}

func combinePortableStationErrors(primary, finalization error) error {
	if primary == nil {
		return finalization
	}
	if finalization == nil {
		return primary
	}
	return fmt.Errorf("%v; finalize exact portable web station: %w", primary, finalization)
}

func runPortableSemanticLeaf[T any](
	ctx context.Context,
	stations *PortableStations,
	job assemblyline.PortableJob,
	decode func(string) (T, error),
) (T, error) {
	var zero T
	if decode == nil {
		return zero, fmt.Errorf("portable web semantic leaf requires one exact decoder")
	}
	result, err := stations.run(ctx, job)
	if err != nil {
		return zero, err
	}
	value, validationErr := decode(result.Candidate)
	if finalizeErr := stations.finalize(
		ctx, job, result, validationErr,
	); finalizeErr != nil {
		return zero, combinePortableStationErrors(validationErr, finalizeErr)
	}
	if validationErr != nil {
		return zero, validationErr
	}
	return value, nil
}
