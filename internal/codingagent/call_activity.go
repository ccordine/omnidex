package codingagent

import "github.com/gryph/omnidex/internal/model"

func (controller *Controller) emitCallActivity(callID int64, call Assignment, status model.AgentCallActivityStatus) error {
	activity := model.AgentCallActivity{Schema: 1, CallID: callID, Role: string(call.Role), Status: status}
	if call.Task != nil {
		activity.TaskID = call.Task.ID
	}
	if err := activity.Validate(); err != nil {
		return err
	}
	if controller.CallActivity != nil {
		controller.CallActivity(activity)
	}
	return nil
}
