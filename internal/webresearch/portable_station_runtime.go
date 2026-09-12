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
) (T, int, error) {
	var zero T
	if stations == nil || stations.runtime.Resolve == nil {
		return zero, 0, fmt.Errorf("portable web stations are uninitialized")
	}
	if ctx == nil {
		return zero, 0, fmt.Errorf("portable web station context is nil")
	}
	if err := ctx.Err(); err != nil {
		return zero, 0, err
	}
	if decode == nil {
		return zero, 0, fmt.Errorf("portable web semantic leaf requires one exact decoder")
	}
	var value T
	var decodeErr error
	decoded := 0
	calls, err := stations.runtime.Resolve(
		ctx,
		job,
		func(raw string) error {
			decoded++
			if decoded != 1 {
				return fmt.Errorf("portable web semantic leaf received more than one candidate")
			}
			value, decodeErr = decode(raw)
			return decodeErr
		},
	)
	if err != nil {
		return zero, calls, err
	}
	if decoded != 1 {
		return zero, calls, fmt.Errorf("portable web semantic leaf decoded %d candidates; expected exactly one", decoded)
	}
	if decodeErr != nil {
		return zero, calls, decodeErr
	}
	if calls < 0 || calls > 1 {
		return zero, calls, fmt.Errorf("portable web semantic leaf reported %d calls outside 0..1", calls)
	}
	return value, calls, nil
}
