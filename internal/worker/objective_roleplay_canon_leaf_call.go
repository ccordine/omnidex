package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type roleplayCanonRawLeafDecoder func(string) (any, error)

type roleplayCanonRawLeafCall func(
	context.Context,
	string,
	assemblyline.PortableJob,
	roleplayCanonRawLeafDecoder,
) (any, objectiveStationReceipt, error)

func callRoleplayCanonRawLeaf[T any](
	ctx context.Context,
	call roleplayCanonRawLeafCall,
	subject string,
	job assemblyline.PortableJob,
	decode func(string) (T, error),
) (T, objectiveStationReceipt, error) {
	var zero T
	if call == nil || decode == nil {
		return zero, objectiveStationReceipt{}, fmt.Errorf(
			"roleplay canon raw semantic leaf call is unavailable",
		)
	}
	value, receipt, err := call(
		ctx,
		subject,
		job,
		func(raw string) (any, error) { return decode(raw) },
	)
	if err != nil {
		return zero, receipt, err
	}
	typed, ok := value.(T)
	if !ok {
		return zero, receipt, fmt.Errorf(
			"roleplay canon raw semantic leaf %s returned an invalid code projection",
			subject,
		)
	}
	return typed, receipt, nil
}
