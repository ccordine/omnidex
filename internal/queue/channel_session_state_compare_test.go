package queue

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestSessionPollingComparesActualRetainedState(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	messageID := int64(7)
	turnID := LifecycleOperationID("lifecycle_operation_turn")
	controlID := LifecycleOperationID("lifecycle_operation_control")
	state := ChannelSessionState{
		ChannelID: "chat-example", WorkspaceRoot: "/tmp/session", WorkspaceIdentity: "directory_1_7",
		ChannelUpdatedAt: now, LatestMessageID: &messageID, LatestTurnOperationID: &turnID,
		LatestControlOperationID: &controlID,
		LatestJob:                &ChannelSessionJobState{ID: 11, Generation: 2, Status: model.JobStatusRunning, UpdatedAt: now},
	}
	same := state
	messageCopy, turnCopy, controlCopy := messageID, turnID, controlID
	jobCopy := *state.LatestJob
	same.LatestMessageID, same.LatestTurnOperationID, same.LatestControlOperationID = &messageCopy, &turnCopy, &controlCopy
	same.LatestJob = &jobCopy
	same.ChannelUpdatedAt = now.In(time.FixedZone("different-zone", -5*60*60))
	if !state.Equal(same) {
		t.Fatal("equal persisted values depended on pointer or timezone representation")
	}
	for name, mutate := range map[string]func(*ChannelSessionState){
		"channel":        func(value *ChannelSessionState) { value.ChannelID = "chat-other" },
		"root":           func(value *ChannelSessionState) { value.WorkspaceRoot = "/tmp/other" },
		"directory":      func(value *ChannelSessionState) { value.WorkspaceIdentity = "directory_1_8" },
		"channel update": func(value *ChannelSessionState) { value.ChannelUpdatedAt = now.Add(time.Second) },
		"message":        func(value *ChannelSessionState) { id := int64(8); value.LatestMessageID = &id },
		"turn": func(value *ChannelSessionState) {
			id := LifecycleOperationID("lifecycle_operation_next_turn")
			value.LatestTurnOperationID = &id
		},
		"control":      func(value *ChannelSessionState) { value.LatestControlOperationID = nil },
		"job identity": func(value *ChannelSessionState) { value.LatestJob.ID++ },
		"generation":   func(value *ChannelSessionState) { value.LatestJob.Generation++ },
		"status":       func(value *ChannelSessionState) { value.LatestJob.Status = model.JobStatusCompleted },
		"job update":   func(value *ChannelSessionState) { value.LatestJob.UpdatedAt = now.Add(time.Second) },
		"absent job":   func(value *ChannelSessionState) { value.LatestJob = nil },
	} {
		t.Run(name, func(t *testing.T) {
			changed := state
			job := *state.LatestJob
			changed.LatestJob = &job
			mutate(&changed)
			if state.Equal(changed) || changed.Equal(state) {
				t.Fatal("polling missed changed persisted state")
			}
		})
	}
}
