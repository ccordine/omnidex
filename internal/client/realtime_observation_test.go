package client

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestRealtimeAgentCallRequiresExactChannelAndJournalBinding(t *testing.T) {
	activity := &model.AgentCallActivity{Schema: 1, CallID: 29, Role: "execute", TaskID: "task-a", Status: model.AgentCallActivityStarted}
	valid := RealtimeEvent{ID: 1, JobID: 1, StepID: 2, Attempt: 3, EventName: RealtimeJobRuntimeEvent, ChannelID: "channel", RuntimeEvent: "agent_call_activity", AgentCallActivity: activity}
	if err := valid.validate("channel"); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RealtimeEvent){
		func(event *RealtimeEvent) { event.ChannelID = "foreign" },
		func(event *RealtimeEvent) { event.AgentCallActivity = nil },
		func(event *RealtimeEvent) { event.RuntimeEvent = "step_start" },
		func(event *RealtimeEvent) { event.FilePath = "source.go" },
		func(event *RealtimeEvent) { event.Detail = "private frame" },
		func(event *RealtimeEvent) {
			invalid := *activity
			invalid.TaskID = ""
			event.AgentCallActivity = &invalid
		},
		func(event *RealtimeEvent) {
			event.EventName = RealtimeJobProgress
			event.Phase = "state_changed"
			event.Summary = "Working"
		},
	} {
		event := valid
		mutate(&event)
		if err := event.validate("channel"); err == nil {
			t.Fatalf("misbound handoff accepted: %+v", event)
		}
	}
}

func TestRealtimeWorkspaceObservationPreservesExactPathAndOperation(t *testing.T) {
	for _, fixture := range []struct{ kind, operation, file string }{
		{"workspace_directory_read", "list", "."},
		{"workspace_directory_read", "list", "src"},
		{"workspace_file_read", "read", "src/main.go"},
	} {
		event := RealtimeEvent{ID: 1, JobID: 1, StepID: 2, Attempt: 3, EventName: RealtimeJobRuntimeEvent, ChannelID: "channel", RuntimeEvent: fixture.kind, FileOperation: fixture.operation, FilePath: fixture.file}
		if err := event.validate("channel"); err != nil {
			t.Fatalf("actual observation rejected: %+v: %v", event, err)
		}
		for _, mutate := range []func(*RealtimeEvent){
			func(event *RealtimeEvent) { event.FileOperation = "delete" },
			func(event *RealtimeEvent) { event.FileSourcePath = "old.go" },
			func(event *RealtimeEvent) { event.FilePath = "../outside" },
			func(event *RealtimeEvent) { event.FilePath = "src/../main.go" },
		} {
			invalid := event
			mutate(&invalid)
			if err := invalid.validate("channel"); err == nil {
				t.Fatalf("invalid observation accepted: %+v", invalid)
			}
		}
	}
}
