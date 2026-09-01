package worker

import (
	"context"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

type exactStationDeadlineProbe struct {
	deadline time.Time
	ctx      context.Context
}

func (probe *exactStationDeadlineProbe) GeneratePreparedExact(
	ctx context.Context,
	_ llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	deadline, present := ctx.Deadline()
	if !present {
		return llm.PreparedGeneration{}, context.DeadlineExceeded
	}
	probe.deadline = deadline
	probe.ctx = ctx
	return llm.PreparedGeneration{}, nil
}

func TestExactStationDirectProviderCallHasThirtyMinuteHardDeadline(t *testing.T) {
	t.Parallel()
	probe := &exactStationDeadlineProbe{}
	earliest := time.Now().Add(llm.MaximumModelRequestDuration)
	if _, err := generatePreparedExactWithinMaximumDuration(
		context.Background(), probe, llm.PreparedModel{},
	); err != nil {
		t.Fatal(err)
	}
	latest := time.Now().Add(llm.MaximumModelRequestDuration)
	if probe.deadline.Before(earliest) || probe.deadline.After(latest) {
		t.Fatalf(
			"provider deadline=%s want within [%s,%s]",
			probe.deadline, earliest, latest,
		)
	}
	select {
	case <-probe.ctx.Done():
	default:
		t.Fatal("per-call provider context was not released after the call")
	}
}
