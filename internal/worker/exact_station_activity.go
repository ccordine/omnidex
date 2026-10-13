package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

// The claimed station owns this observer. Display never supplies semantic
// context, consumes the station result or grants native tool access.
func (s *Service) exactStationPublicActivity(ctx context.Context, authority model.StepAttemptAuthority) func(model.AgentActivity) error {
	if s == nil || s.runtimeEventSink == nil {
		return nil
	}
	return func(activity model.AgentActivity) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := activity.Validate(); err != nil {
			return err
		}
		if activity.Source != model.AgentActivityNative || activity.Role != "plan" || activity.TaskID != "" ||
			activity.Kind != model.AgentActivityMessage && activity.Kind != model.AgentActivityReasoningSummary {
			return fmt.Errorf("semantic station activity must be public text from its exact native inference")
		}
		s.emitRuntimeEvent(RuntimeEvent{JobID: authority.JobID, StepID: authority.StepID, Attempt: authority.Attempt,
			Kind: "agent_activity", AgentActivity: &activity})
		return nil
	}
}
