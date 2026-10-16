package model

import "testing"

func TestAgentCallActivityRequiresExactJournalAndRoleBinding(t *testing.T) {
	for _, role := range []string{"plan", "execute", "review", "review_response"} {
		valid := AgentCallActivity{Schema: 1, CallID: 17, Role: role, Status: AgentCallActivityStarted}
		if role == "execute" || role == "review" || role == "review_response" {
			valid.TaskID = "task-a"
		}
		if err := valid.Validate(); err != nil {
			t.Fatal(err)
		}
		for _, mutate := range []func(*AgentCallActivity){
			func(activity *AgentCallActivity) { activity.CallID = 0 },
			func(activity *AgentCallActivity) { activity.Schema = 2 },
			func(activity *AgentCallActivity) { activity.Role = "worker" },
			func(activity *AgentCallActivity) { activity.Status = "approved" },
			func(activity *AgentCallActivity) { activity.TaskID = " task-a " },
			func(activity *AgentCallActivity) {
				if activity.TaskID == "" {
					activity.TaskID = "foreign-task"
				} else {
					activity.TaskID = ""
				}
			},
		} {
			invalid := valid
			mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatalf("invalid call lifecycle accepted: %+v", invalid)
			}
		}
	}
}
