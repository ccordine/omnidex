package codingagent

import (
	"context"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/model"
)

func (controller *Controller) executeChecks(ctx context.Context, call Assignment, checks []Check) ([]CheckResult, error) {
	taskID := ""
	if call.Task != nil {
		taskID = call.Task.ID
	}
	current, active := 0, false
	return controller.Verifier.ExecuteChecks(ctx, call, checks, func(activity model.AgentActivity) error {
		if err := activity.Validate(); err != nil {
			return err
		}
		if activity.Source != model.AgentActivityVerification || activity.Role != string(call.Role) || activity.TaskID != taskID || current >= len(checks) || activity.CheckID != checks[current].ID {
			return fmt.Errorf("verification activity changed its frozen check assignment")
		}
		if activity.Kind == model.AgentActivityCommand {
			expected := activity
			expected.CWD, expected.Argv, expected.Truncated = call.Root, checks[current].Argv, false
			expected = model.BoundAgentActivityCommand(expected)
			if activity.CWD != expected.CWD || activity.Truncated != expected.Truncated || !slices.Equal(activity.Argv, expected.Argv) {
				return fmt.Errorf("verification activity changed its exact command or directory")
			}
			if activity.Status == model.AgentActivityStarted {
				if active {
					return fmt.Errorf("verification activity started overlapping checks")
				}
				active = true
			} else {
				if !active {
					return fmt.Errorf("verification activity completed an unstarted check")
				}
				active = false
				current++
			}
		} else if !active {
			return fmt.Errorf("verification output belongs to an inactive check")
		}
		if controller.Activity != nil {
			activity.Argv = append([]string(nil), activity.Argv...)
			controller.Activity(activity)
		}
		return nil
	})
}
