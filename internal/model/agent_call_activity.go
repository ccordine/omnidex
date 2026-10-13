package model

import (
	"fmt"
	"strings"
)

type AgentCallActivityStatus string

const (
	AgentCallActivityStarted   AgentCallActivityStatus = "started"
	AgentCallActivityCompleted AgentCallActivityStatus = "completed"
	AgentCallActivityFailed    AgentCallActivityStatus = "failed"
)

// AgentCallActivity observes the existing queue journal's native call lifecycle.
// CallID identifies its call_opened receipt; it confers no execution authority.
type AgentCallActivity struct {
	Schema int                     `json:"schema"`
	CallID int64                   `json:"callID"`
	Role   string                  `json:"role"`
	TaskID string                  `json:"taskID,omitempty"`
	Status AgentCallActivityStatus `json:"status"`
}

func (activity AgentCallActivity) Validate() error {
	if activity.Schema != 1 || activity.CallID < 1 {
		return fmt.Errorf("agent call activity requires its supported schema and exact journal identity")
	}
	switch activity.Role {
	case "plan", "overall_review":
		if activity.TaskID != "" {
			return fmt.Errorf("global agent call activity cannot carry a task identity")
		}
	case "execute", "review":
		if activity.TaskID == "" || !activityText(activity.TaskID, 256) || strings.TrimSpace(activity.TaskID) != activity.TaskID {
			return fmt.Errorf("task agent call activity lacks its exact task identity")
		}
	default:
		return fmt.Errorf("agent call activity has an unregistered role")
	}
	switch activity.Status {
	case AgentCallActivityStarted, AgentCallActivityCompleted, AgentCallActivityFailed:
		return nil
	default:
		return fmt.Errorf("agent call activity has an unregistered status")
	}
}
