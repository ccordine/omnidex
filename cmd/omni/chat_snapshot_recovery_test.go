package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestSnapshotFollowsRecoveredJobWithoutRepeatingMessages(t *testing.T) {
	for _, observeRunning := range []bool{true, false} {
		name := "resume between snapshots"
		if observeRunning {
			name = "observe resumed execution"
		}
		t.Run(name, func(t *testing.T) {
			session := newChatOperationTestSession(t, "http://localhost")
			console, output := planReviewTestConsole(t)
			session.renderer.console = console
			snapshot := operationGuardActiveSnapshot(t, session)
			message := snapshot.Messages[0]
			states := []model.ChannelMessageTurnState{
				{JobID: 41, Status: model.JobStatusFailed, Error: "Execution interrupted."},
			}
			if observeRunning {
				states = append(states, model.ChannelMessageTurnState{JobID: 41, Status: model.JobStatusRunning})
			}
			states = append(states,
				model.ChannelMessageTurnState{JobID: 41, Status: model.JobStatusFailed, Error: "Review is incomplete."},
				model.ChannelMessageTurnState{JobID: 41, Status: model.JobStatusCompleted},
			)
			for index, state := range states {
				state.UpdatedAt = message.CreatedAt.Add(time.Duration(index+1) * time.Second)
				snapshot.Messages[0].Turn = &state
				snapshot.State.LatestJob = &model.ChannelSessionJobState{ID: 41, Generation: 1, Status: state.Status, UpdatedAt: state.UpdatedAt}
				if state.Status == model.JobStatusRunning {
					snapshot.ActiveJob = operationGuardActiveSnapshot(t, session).ActiveJob
				} else {
					snapshot.ActiveJob = nil
				}
				if err := session.reconcileSnapshot(snapshot, index == 0); err != nil {
					t.Fatalf("reconcile %s: %v", state.Status, err)
				}
				if got := session.messages[message.ID].Turn; *got != state {
					t.Fatalf("retained job state = %#v, want %#v", got, state)
				}
				if !session.serverState.Equal(snapshot.State) {
					t.Fatal("polling state did not advance after recovery")
				}
				length := output.Len()
				for range 3 {
					if err := session.reconcileSnapshot(snapshot, false); err != nil {
						t.Fatalf("repeat snapshot: %v", err)
					}
				}
				if output.Len() != length {
					t.Fatalf("unchanged snapshot repeated output: %s", output.String()[length:])
				}
			}
			for _, text := range []string{message.Content, "Execution interrupted.", "Review is incomplete.", "job 41 · completed"} {
				if count := strings.Count(output.String(), text); count != 1 {
					t.Fatalf("%q appeared %d times: %s", text, count, output.String())
				}
			}
			fresh := newChatOperationTestSession(t, "http://localhost")
			fresh.channel = session.channel
			fresh.renderer.console, _ = planReviewTestConsole(t)
			if err := fresh.reconcileSnapshot(client.ChatSessionSnapshot{Channel: fresh.channel, Messages: snapshot.Messages}, true); err != nil {
				t.Fatalf("reconnect: %v", err)
			}
		})
	}
}

func TestRecoveredMessagePreservesContentIdentityAndTime(t *testing.T) {
	now := time.Now().UTC()
	previous := model.ChannelMessage{ID: 335, ChannelID: "channel", Role: model.ChannelMessageRoleUser,
		Content: "Retain this objective.", CreatedAt: now,
		Turn: &model.ChannelMessageTurnState{JobID: 42, Status: model.JobStatusFailed, Error: "Interrupted.", UpdatedAt: now}}
	for _, test := range []struct {
		name   string
		mutate func(*model.ChannelMessage)
	}{
		{"content", func(message *model.ChannelMessage) { message.Content = "Other objective." }},
		{"identity", func(message *model.ChannelMessage) { message.Turn.JobID++ }},
		{"binding", func(message *model.ChannelMessage) { message.Turn = nil }},
		{"stale state", func(message *model.ChannelMessage) { message.Turn.UpdatedAt = now.Add(-time.Second) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := previous
			state := *previous.Turn
			current.Turn = &state
			state.Status, state.Error, state.UpdatedAt = model.JobStatusRunning, "", now.Add(time.Second)
			test.mutate(&current)
			if _, err := changedMessageTurn(previous, current); err == nil {
				t.Fatal("invalid message mutation accepted during recovery")
			}
		})
	}
}
