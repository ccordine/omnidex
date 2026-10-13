package client

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestRealtimeAgentActivityRequiresExactChannelAndProjection(t *testing.T) {
	activity := &model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityMessage, Status: model.AgentActivityDelta, ThreadID: "thread", TurnID: "turn", ItemID: "message", Role: "review", TaskID: "task", Text: "Checking the saved change."}
	valid := RealtimeEvent{ID: 1, JobID: 1, StepID: 2, Attempt: 1, EventName: RealtimeJobRuntimeEvent, ChannelID: "channel", RuntimeEvent: "agent_activity", AgentActivity: activity}
	if err := valid.validate("channel"); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RealtimeEvent){
		func(event *RealtimeEvent) { event.ChannelID = "foreign" },
		func(event *RealtimeEvent) { event.AgentActivity = nil },
		func(event *RealtimeEvent) { event.RuntimeEvent = "step_start" },
		func(event *RealtimeEvent) { event.FilePath = "source.go" },
		func(event *RealtimeEvent) { event.Detail = "private raw frame" },
		func(event *RealtimeEvent) {
			event.EventName = RealtimeJobProgress
			event.Phase = "state_changed"
			event.Summary = "Working"
		},
	} {
		event := valid
		mutate(&event)
		if err := event.validate("channel"); err == nil {
			t.Fatalf("misbound agent activity accepted: %+v", event)
		}
	}
}
