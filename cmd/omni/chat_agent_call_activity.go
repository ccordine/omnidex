package main

import "github.com/gryph/omnidex/internal/model"

func (renderer chatRenderer) agentCallActivity(activity model.AgentCallActivity) error {
	if (renderer.verbosity == chatQuiet || renderer.verbosity == chatNormal) && activity.Status != model.AgentCallActivityFailed {
		return nil
	}
	if renderer.verbosity == chatDebug {
		return renderer.system("agent call %d · role %s · task %q · %s", activity.CallID, activity.Role, activity.TaskID, activity.Status)
	}
	return renderer.system("%s · %s", agentRoleLabel(activity.Role), activity.Status)
}
