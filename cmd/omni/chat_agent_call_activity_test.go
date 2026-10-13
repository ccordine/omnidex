package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestAgentCallLifecycleShowsTypedActualRoleAndStatus(t *testing.T) {
	for _, role := range []string{"plan", "execute", "review", "overall_review"} {
		for _, status := range []model.AgentCallActivityStatus{model.AgentCallActivityStarted, model.AgentCallActivityCompleted, model.AgentCallActivityFailed} {
			t.Run(role+"/"+string(status), func(t *testing.T) {
				console, output := planReviewTestConsole(t)
				renderer := chatRenderer{console: console}
				activity := model.AgentCallActivity{Schema: 1, CallID: 73, Role: role, Status: status}
				if role == "execute" || role == "review" {
					activity.TaskID = "actual-task"
				}
				if err := renderer.realtime(client.RealtimeEvent{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12,
					RuntimeEvent: "agent_call_activity", AgentCallActivity: &activity}); err != nil {
					t.Fatal(err)
				}
				labels := map[string]string{"plan": "Planning", "execute": "Executing task", "review": "Reviewing", "overall_review": "Overall review"}
				if !strings.Contains(output.String(), labels[role]) || !strings.Contains(output.String(), string(status)) {
					t.Fatalf("actual native role handoff was hidden: %q", output.String())
				}
				if strings.Contains(output.String(), "agent_call_activity") || strings.Contains(output.String(), "role=") || strings.Contains(output.String(), "callID") {
					t.Fatalf("typed role handoff exposed raw transport metadata: %q", output.String())
				}
			})
		}
	}
}

func TestQuietAgentCallLifecycleKeepsFailuresWithoutRoutineHandoffs(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console, verbosity: chatQuiet}
	for _, status := range []model.AgentCallActivityStatus{model.AgentCallActivityStarted, model.AgentCallActivityCompleted} {
		activity := model.AgentCallActivity{Schema: 1, CallID: 73, Role: "plan", Status: status}
		if err := renderer.realtime(client.RealtimeEvent{EventName: client.RealtimeJobRuntimeEvent,
			RuntimeEvent: "agent_call_activity", AgentCallActivity: &activity}); err != nil {
			t.Fatal(err)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("quiet output exposed routine native call handoffs: %q", output.String())
	}
	activity := model.AgentCallActivity{Schema: 1, CallID: 73, Role: "plan", Status: model.AgentCallActivityFailed}
	if err := renderer.realtime(client.RealtimeEvent{EventName: client.RealtimeJobRuntimeEvent,
		RuntimeEvent: "agent_call_activity", AgentCallActivity: &activity}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Planning") || !strings.Contains(output.String(), "failed") {
		t.Fatalf("quiet output hid actual native call failure: %q", output.String())
	}
}
