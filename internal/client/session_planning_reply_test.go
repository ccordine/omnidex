package client

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCapturedPlanningRepliesKeepValidActiveCLISnapshot(t *testing.T) {
	route := codingagent.Route{Provider: "codex", Model: "fixture"}
	metadata, _ := json.Marshal(map[string]any{"coding_execution": codingagent.Configuration{
		Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route,
	}})
	for _, name := range []string{"expense", "wordcounter"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile("../../testdata/planning-gauntlet/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture struct {
				Instruction string `json:"instruction"`
				Responses   []struct {
					OwnerTurn string `json:"owner_turn"`
					Response  string `json:"response"`
				} `json:"responses"`
			}
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			job := model.Job{ID: 41, Status: model.JobStatusRunning, Instruction: fixture.Instruction, Metadata: metadata}
			messages := []model.ChannelMessage{
				{ID: 1, Role: model.ChannelMessageRoleUser, Content: job.Instruction, Turn: &model.ChannelMessageTurnState{JobID: job.ID, Status: job.Status}},
				{ID: 2, Role: model.ChannelMessageRoleAssistant, Content: fixture.Responses[0].Response},
				{ID: 3, Role: model.ChannelMessageRoleUser, Content: fixture.Responses[1].OwnerTurn},
				{ID: 4, Role: model.ChannelMessageRoleAssistant, Content: fixture.Responses[1].Response},
			}
			if err := validateChatSessionActiveTurn(messages, &job, 1); err != nil {
				t.Fatalf("server-accepted planning reply broke CLI: %v", err)
			}
			messages[2].Turn = &model.ChannelMessageTurnState{JobID: 42, Status: model.JobStatusRunning}
			if err := validateChatSessionActiveTurn(messages, &job, 1); err == nil {
				t.Fatal("second active job was admitted as a planning reply")
			}
		})
	}
}

func TestQueuedPlanningResolutionRemainsAValidSessionTurn(t *testing.T) {
	for _, status := range []string{model.JobStatusPending, model.JobStatusRunning} {
		turn := model.ChannelSessionTurn{OperationID: "lifecycle_operation_planning_reply", Disposition: model.ChannelSessionTurnQueued,
			Text: "Keep exactly that scope and order. No implementation yet.", JobID: 1, Generation: 1, Status: status,
			CreatedAt: time.Date(2026, 10, 16, 19, 29, 30, 0, time.UTC), MessageState: model.ChannelSessionMessagePlanning}
		snapshot := ChatSessionSnapshot{Turns: []model.ChannelSessionTurn{turn}}
		if err := validateChatSessionTurns(snapshot); err != nil {
			t.Fatalf("server-resolved planning message broke the CLI snapshot: %v", err)
		}
		for _, invalid := range []func(*model.ChannelSessionTurn){
			func(turn *model.ChannelSessionTurn) { turn.MessageState = "unregistered" },
			func(turn *model.ChannelSessionTurn) { turn.JobID = 0 },
			func(turn *model.ChannelSessionTurn) { turn.Generation = 0 },
			func(turn *model.ChannelSessionTurn) { turn.Status = model.JobStatusWaiting },
		} {
			broken := turn
			invalid(&broken)
			if err := validateChatSessionTurns(ChatSessionSnapshot{Turns: []model.ChannelSessionTurn{broken}}); err == nil {
				t.Fatal("planning resolution weakened message state or exact admitted authority")
			}
		}
	}
}
