package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type objectiveDatabaseRawLeafDecoder func(string) (any, error)

type objectiveDatabaseRawLeafCall func(
	context.Context,
	string,
	assemblyline.PortableJob,
	objectiveDatabaseRawLeafDecoder,
) (any, int, error)

func (adapter portableObjectiveDatabaseStations) rawLeafCall(
	owner station.ID,
	resolveModel func() (string, error),
	ledger *objectiveDatabaseRawLeafCallLedger,
) objectiveDatabaseRawLeafCall {
	return func(
		ctx context.Context,
		subject string,
		job assemblyline.PortableJob,
		decode objectiveDatabaseRawLeafDecoder,
	) (any, int, error) {
		value, receipt, err := runObjectivePortableRawLeafStation[any](
			ctx, adapter.runtime, subject, job, owner, resolveModel,
			objectiveRawLeafDecoder[any](decode),
			func(any) error { return nil },
		)
		if err != nil {
			return value, receipt.Calls, err
		}
		if ledger == nil {
			return nil, receipt.Calls, fmt.Errorf(
				"database raw semantic leaf %s has no receipt ledger", subject,
			)
		}
		if err := ledger.record(subject, receipt); err != nil {
			return nil, receipt.Calls, err
		}
		return value, receipt.Calls, nil
	}
}

func callObjectiveDatabaseRawLeaf[T any](
	ctx context.Context,
	call objectiveDatabaseRawLeafCall,
	subject string,
	job assemblyline.PortableJob,
	decode func(string) (T, error),
) (T, int, error) {
	var zero T
	if call == nil || decode == nil {
		return zero, 0, fmt.Errorf("database raw semantic leaf call is unavailable")
	}
	value, calls, err := call(
		ctx, subject, job,
		func(raw string) (any, error) { return decode(raw) },
	)
	if err != nil {
		return zero, calls, err
	}
	typed, ok := value.(T)
	if !ok {
		return zero, calls, fmt.Errorf("database raw semantic leaf %s returned an invalid code projection", subject)
	}
	return typed, calls, nil
}
