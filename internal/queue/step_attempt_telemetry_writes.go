package queue

import (
	"context"

	"github.com/gryph/omnidex/internal/model"
)

func (r *Repository) RecordLLMContextUsageByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	record LLMContextUsageRecord,
) error {
	if record.JobID != authority.JobID || record.StepID != authority.StepID {
		return staleStepAttemptError(authority, "LLM context usage disagrees with step attempt", nil)
	}
	return underActiveStepAttemptWriteFence(ctx, r, authority, "record LLM context usage", func() error {
		return r.RecordLLMContextUsage(ctx, record)
	})
}

func (r *Repository) RecordTelemetryModelCallByStepAttempt(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	record TelemetryModelCallRecord,
) error {
	return underActiveStepAttemptWriteFence(ctx, r, authority, "record telemetry model call", func() error {
		return r.RecordTelemetryModelCall(ctx, record)
	})
}
