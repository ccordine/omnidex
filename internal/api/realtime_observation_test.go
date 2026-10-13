package api

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestRealtimeAgentCallPreservesItsJournalIdentity(t *testing.T) {
	activity := &model.AgentCallActivity{Schema: 1, CallID: 29, Role: "review", TaskID: "task-a", Status: model.AgentCallActivityStarted}
	event := JobRuntimeEvent{JobID: 1, ChannelID: "call-channel", StepID: 2, Attempt: 3, RuntimeEvent: "agent_call_activity", AgentCallActivity: activity}
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
	if !reflect.DeepEqual(received.AgentCallActivity, activity) || received.JobID != 1 || received.StepID != 2 || received.Attempt != 3 || received.ChannelID != event.ChannelID {
		t.Fatalf("journal-bound handoff was lost: %+v", received)
	}
}

func TestRealtimeRejectsUnboundAndMixedAgentCallObservations(t *testing.T) {
	activity := &model.AgentCallActivity{Schema: 1, CallID: 29, Role: "plan", Status: model.AgentCallActivityStarted}
	valid := JobRuntimeEvent{JobID: 1, StepID: 2, Attempt: 3, RuntimeEvent: "agent_call_activity", AgentCallActivity: activity}
	for _, mutate := range []func(*JobRuntimeEvent){
		func(event *JobRuntimeEvent) { event.AgentCallActivity = nil },
		func(event *JobRuntimeEvent) { event.RuntimeEvent = "step_start" },
		func(event *JobRuntimeEvent) { event.Detail = "unrelated text" },
		func(event *JobRuntimeEvent) { event.FilePath = "source.go" },
		func(event *JobRuntimeEvent) {
			invalid := *activity
			invalid.CallID = 0
			event.AgentCallActivity = &invalid
		},
		func(event *JobRuntimeEvent) {
			event.AgentActivity = &model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityMessage, Status: model.AgentActivityDelta, Role: "plan", ThreadID: "thread", TurnID: "turn", ItemID: "message", Text: "public text"}
		},
	} {
		event := valid
		mutate(&event)
		server := &Server{realtimeHub: NewRealtimeHub()}
		if err := server.PublishJobRuntimeEvent(event); err == nil || server.realtimeHub.nextMessageID != 0 {
			t.Fatalf("invalid handoff was published: %+v", event)
		}
	}
}

func TestRealtimeWorkspaceObservationRequiresExactSuccessfulOperation(t *testing.T) {
	for _, fixture := range []struct{ kind, operation, file string }{
		{"workspace_directory_read", "list", "."},
		{"workspace_directory_read", "list", "src"},
		{"workspace_file_read", "read", "src/main.go"},
	} {
		event := JobRuntimeEvent{JobID: 1, StepID: 2, Attempt: 3, RuntimeEvent: fixture.kind, FileOperation: fixture.operation, FilePath: fixture.file}
		if err := event.validate(); err != nil {
			t.Fatalf("actual observation rejected: %+v: %v", event, err)
		}
		for _, mutate := range []func(*JobRuntimeEvent){
			func(event *JobRuntimeEvent) { event.FileOperation = "replace" },
			func(event *JobRuntimeEvent) { event.FileSourcePath = "previous.go" },
			func(event *JobRuntimeEvent) { event.FilePath = "../outside" },
			func(event *JobRuntimeEvent) { event.FilePath = "src/../main.go" },
		} {
			invalid := event
			mutate(&invalid)
			if err := invalid.validate(); err == nil {
				t.Fatalf("observation acquired invalid file authority: %+v", invalid)
			}
		}
	}
}
