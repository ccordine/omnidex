package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func TestUserIntentEchoReconcilesIdentityAndSurvivesReconnect(t *testing.T) {
	for _, kind := range []string{"message", "queued turn", "control"} {
		t.Run(kind, func(t *testing.T) {
			session := newChatOperationTestSession(t, "http://localhost")
			console, output := planReviewTestConsole(t)
			session.renderer.console = console
			const text = "Keep café 世界 exactly."
			snapshot := client.ChatSessionSnapshot{Channel: session.channel}
			add := func(index int) sessionintent.Receipt {
				id := model.LifecycleOperationID(fmt.Sprintf("operation-%d", index))
				receipt := sessionintent.Receipt{}
				switch kind {
				case "message":
					receipt.MessageID, receipt.TurnOperationID = int64(index), id
					snapshot.Messages = append(snapshot.Messages, model.ChannelMessage{ID: int64(index), ChannelID: session.channel.ID, Role: model.ChannelMessageRoleUser, Content: text,
						Turn: &model.ChannelMessageTurnState{JobID: int64(index), Status: model.JobStatusRunning, UpdatedAt: time.Now()}})
					snapshot.Turns = append(snapshot.Turns, model.ChannelSessionTurn{OperationID: id, JobID: int64(index), Disposition: model.ChannelSessionTurnEnqueued, Text: text})
				case "queued turn":
					receipt.TurnOperationID = id
					snapshot.Turns = append(snapshot.Turns, model.ChannelSessionTurn{OperationID: id, JobID: 1, Disposition: model.ChannelSessionTurnQueued, MessageState: model.ChannelSessionMessagePending, Text: text})
				case "control":
					receipt.ControlOperationID = id
					snapshot.Controls = append(snapshot.Controls, model.ChannelSessionControl{OperationID: id, Kind: model.ChannelSessionControlCancel, Text: text})
				}
				return receipt
			}
			for index := 1; index <= 3; index++ {
				receipt := add(index)
				if index != 2 { // Another client may submit the very same text.
					if err := console.WriteOutput("you> " + text + "\n"); err != nil {
						t.Fatal(err)
					}
					session.rememberUserIntentEcho(receipt, text)
				}
				for range 2 { // Snapshot/realtime refresh must not echo it again.
					if err := session.reconcileSnapshot(snapshot, false); err != nil {
						t.Fatal(err)
					}
				}
				if got := strings.Count(output.String(), text); got != index {
					t.Fatalf("after submission %d: %d echoes\n%s", index, got, output.String())
				}
				if len(session.localEchoes) != 0 {
					t.Fatal("observed echo identities were retained")
				}
			}
			fresh := newChatOperationTestSession(t, "http://localhost")
			fresh.channel = session.channel
			freshConsole, restored := planReviewTestConsole(t)
			fresh.renderer.console = freshConsole
			if err := fresh.reconcileSnapshot(snapshot, true); err != nil {
				t.Fatal(err)
			}
			if strings.Count(restored.String(), text) != 3 {
				t.Fatalf("reconnect lost or duplicated persisted turns: %s", restored.String())
			}
		})
	}
}

func TestUserIntentEchoDoesNotHideDifferentPersistedContent(t *testing.T) {
	session := newChatOperationTestSession(t, "http://localhost")
	console, output := planReviewTestConsole(t)
	session.renderer.console = console
	session.rememberUserIntentEcho(sessionintent.Receipt{MessageID: 1}, "yes")
	snapshot := client.ChatSessionSnapshot{Channel: session.channel, Messages: []model.ChannelMessage{{
		ID: 1, ChannelID: session.channel.ID, Role: model.ChannelMessageRoleUser, Content: "The original approved request.",
	}}}
	if err := session.reconcileSnapshot(snapshot, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "The original approved request.") {
		t.Fatal("different committed content was hidden")
	}
}

func TestCanceledMessageDoesNotRepeatItsControlReason(t *testing.T) {
	for _, controlText := range []string{"Yes, cancel it.", "A different cancellation reason.", ""} {
		t.Run(controlText, func(t *testing.T) {
			session := newChatOperationTestSession(t, "http://localhost")
			console, output := planReviewTestConsole(t)
			session.renderer.console = console
			const reason = "Yes, cancel it."
			snapshot := client.ChatSessionSnapshot{Channel: session.channel, Messages: []model.ChannelMessage{{
				ID: 1, ChannelID: session.channel.ID, Role: model.ChannelMessageRoleUser, Content: "Initial request.",
				Turn: &model.ChannelMessageTurnState{JobID: 1, Status: model.JobStatusCanceled, Error: reason},
			}}}
			if controlText != "" {
				snapshot.Controls = []model.ChannelSessionControl{{OperationID: "cancel", JobID: 1, Kind: model.ChannelSessionControlCancel, Text: controlText}}
			}
			if err := session.reconcileSnapshot(snapshot, true); err != nil {
				t.Fatal(err)
			}
			if strings.Count(output.String(), reason) != 1 || !strings.Contains(output.String(), "job 1 · canceled") {
				t.Fatalf("cancellation reason was duplicated or lost: %s", output.String())
			}
			if session.messages[1].Turn.Error != reason {
				t.Fatal("presentation changed the authoritative reason")
			}
			if controlText == reason && strings.Contains(output.String(), "reason>") {
				t.Fatal("omitted echo left an empty reason label")
			}
		})
	}
}
