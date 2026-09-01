package webresearch

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runPortableSemanticLeaf[T any](
	ctx context.Context,
	stations *PortableStations,
	job assemblyline.PortableJob,
	decode func(string) (T, error),
) (T, SemanticCallReceipt, error) {
	var zero T
	if stations == nil || stations.runtime.Resolve == nil {
		return zero, SemanticCallReceipt{}, fmt.Errorf("portable web stations are uninitialized")
	}
	if ctx == nil {
		return zero, SemanticCallReceipt{}, fmt.Errorf("portable web station context is nil")
	}
	if err := ctx.Err(); err != nil {
		return zero, SemanticCallReceipt{}, err
	}
	if decode == nil {
		return zero, SemanticCallReceipt{}, fmt.Errorf("portable web semantic leaf requires one exact decoder")
	}
	var value T
	receipt, err := stations.runtime.Resolve(
		ctx,
		job,
		func(raw string) error {
			var decodeErr error
			value, decodeErr = decode(raw)
			return decodeErr
		},
	)
	if err != nil {
		return zero, receipt, err
	}
	if err := ValidateSemanticCallReceipt(
		"portable web semantic leaf", receipt, exactPortableSemanticLeafCalls,
	); err != nil {
		return zero, receipt, err
	}
	return value, receipt, nil
}
