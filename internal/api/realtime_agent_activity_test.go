package api

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestRealtimeAgentActivityPreservesTypedPublicProjection(t *testing.T) {
	activity := &model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityReasoningSummary, Status: model.AgentActivityDelta, ThreadID: "thread", TurnID: "turn", ItemID: "summary", Role: "plan", Text: "Checking the input boundary."}
	event := JobRuntimeEvent{JobID: 1, ChannelID: "activity-channel", StepID: 2, Attempt: 1, RuntimeEvent: "agent_activity", AgentActivity: activity}
	server := &Server{realtimeHub: NewRealtimeHub()}
	topic, err := realtimeChannelTopic(event.ChannelID)
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := server.realtimeHub.Subscribe([]string{topic}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Unsubscribe()
	if err := server.PublishJobRuntimeEvent(event); err != nil {
		t.Fatal(err)
	}
	var received client.RealtimeEvent
	if err := json.Unmarshal(<-subscription.Messages, &received); err != nil {
		t.Fatal(err)
	}
	if received.AgentActivity == nil || !reflect.DeepEqual(received.AgentActivity, activity) || received.JobID != event.JobID || received.ChannelID != event.ChannelID || received.RuntimeEvent != "agent_activity" {
		t.Fatalf("activity lost its typed channel/job binding: %+v", received)
	}
}

func TestRealtimeRejectsMissingMisboundAndOverBoundAgentActivity(t *testing.T) {
	activity := &model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityCommand, Status: model.AgentActivityStarted, ThreadID: "thread", TurnID: "turn", ItemID: "command", Role: "execute", TaskID: "task", Command: "go test ./...", CWD: "/project"}
	for _, event := range []JobRuntimeEvent{
		{JobID: 1, StepID: 2, Attempt: 1, RuntimeEvent: "agent_activity"},
		{JobID: 1, StepID: 2, Attempt: 1, RuntimeEvent: "step_start", AgentActivity: activity},
		{JobID: 1, StepID: 2, Attempt: 1, RuntimeEvent: "agent_activity", AgentActivity: activity, Detail: "raw provider output"},
		{JobID: 1, StepID: 2, Attempt: 1, RuntimeEvent: "agent_activity", AgentActivity: &model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityReasoningSummary, Status: model.AgentActivityDelta, ThreadID: "thread", TurnID: "turn", ItemID: "summary", Role: "plan", Text: string(make([]byte, model.MaxAgentActivityTextBytes+1))}},
	} {
		server := &Server{realtimeHub: NewRealtimeHub()}
		if err := server.PublishJobRuntimeEvent(event); err == nil || server.realtimeHub.nextMessageID != 0 {
			t.Fatalf("invalid projection was published: %+v", event)
		}
	}
}
