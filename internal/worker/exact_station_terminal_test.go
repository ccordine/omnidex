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
	if present {
		return llm.PreparedGeneration{}, context.DeadlineExceeded
	}
	probe.deadline = deadline
	probe.ctx = ctx
	return llm.PreparedGeneration{}, nil
}

func TestExactStationDirectProviderCallHasNoArtificialDeadline(t *testing.T) {
	t.Parallel()
	probe := &exactStationDeadlineProbe{}
	if _, err := generatePreparedExact(
		context.Background(), probe, llm.PreparedModel{},
	); err != nil {
		t.Fatal(err)
	}
	if !probe.deadline.IsZero() {
		t.Fatal("provider gained an artificial deadline")
	}
	select {
	case <-probe.ctx.Done():
	default:
		t.Fatal("per-call provider context was not released after the call")
	}
}
