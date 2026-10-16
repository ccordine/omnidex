package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestCLIChatSnapshotAcceptsExactAgentQuestionPause(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"v3_coding_plan", "v3_coding"} {
		snapshot := agentQuestionSnapshot(t)
		snapshot.ActiveJob.Steps[0].Action = action
		if err := validateChatSessionSnapshot(snapshot, testWorkspaceIdentity, MaxChatSessionMessages); err != nil {
			t.Fatalf("durable %s agent question rejected by CLI chat: %v", action, err)
		}
	}
}

func agentQuestionSnapshot(t *testing.T) ChatSessionSnapshot {
	t.Helper()
	channel := testCLIChannel("/workspace/terminal-question")
	metadata, err := json.Marshal(map[string]any{
		"channel_id": channel.ID, "channel_user_message_id": 1,
		"client_cwd": channel.WorkspaceRoot, "client_workspace_identity": testWorkspaceIdentity,
		"coding_execution": codingagent.Configuration{Mode: codingagent.Agents},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	return ChatSessionSnapshot{
		Channel: channel, WorkspaceIdentity: testWorkspaceIdentity,
		State: model.ChannelSessionState{
			ChannelID: channel.ID, WorkspaceRoot: channel.WorkspaceRoot,
			WorkspaceIdentity: testWorkspaceIdentity, ChannelUpdatedAt: channel.UpdatedAt,
			LatestJob: &model.ChannelSessionJobState{
				ID: 41, Generation: 2, Status: model.JobStatusWaiting, UpdatedAt: now,
			},
		},
		Messages: []model.ChannelMessage{{
			ID: 1, ChannelID: channel.ID, Role: model.ChannelMessageRoleUser,
			Content: "Preserve the requested behavior.", CreatedAt: now,
			Turn: &model.ChannelMessageTurnState{JobID: 41, Status: model.JobStatusWaiting, UpdatedAt: now},
		}},
		Turns: []model.ChannelSessionTurn{}, Controls: []model.ChannelSessionControl{},
		ActiveJob: &model.JobDetails{
			Job: model.Job{
				ID: 41, Instruction: "Preserve the requested behavior.", Pipeline: model.PipelineChat,
				Status: model.JobStatusWaiting, CurrentGeneration: 2, Metadata: metadata,
			},
			Steps: []model.Step{{ID: 11, JobID: 41, Generation: 2, Action: "v3_coding", Status: model.StepStatusWaiting}},
			AgentPause: &model.CodingAgentPause{
				Schema: "omnidex-coding-agent-pause-v1", Generation: 2, StepID: 11, WaitingAttempt: 3,
				Questions: []string{"Which approved format should this task use?"},
			},
		},
	}
}

func TestCLIChatSnapshotRejectsAgentPauseOutsideExactQuestionAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*ChatSessionSnapshot)
	}{
		{name: "wrong schema", change: func(snapshot *ChatSessionSnapshot) { snapshot.ActiveJob.AgentPause.Schema = "unknown" }},
		{name: "wrong generation", change: func(snapshot *ChatSessionSnapshot) { snapshot.ActiveJob.AgentPause.Generation++ }},
		{name: "wrong step", change: func(snapshot *ChatSessionSnapshot) { snapshot.ActiveJob.AgentPause.StepID++ }},
		{name: "missing waiting attempt", change: func(snapshot *ChatSessionSnapshot) { snapshot.ActiveJob.AgentPause.WaitingAttempt = 0 }},
		{name: "missing questions", change: func(snapshot *ChatSessionSnapshot) { snapshot.ActiveJob.AgentPause.Questions = nil }},
		{name: "blank question", change: func(snapshot *ChatSessionSnapshot) { snapshot.ActiveJob.AgentPause.Questions = []string{" \t "} }},
		{name: "NUL question", change: func(snapshot *ChatSessionSnapshot) {
			snapshot.ActiveJob.AgentPause.Questions = []string{"question\x00"}
		}},
		{name: "wrong step action", change: func(snapshot *ChatSessionSnapshot) { snapshot.ActiveJob.Steps[0].Action = "objective_resolve" }},
		{name: "running step", change: func(snapshot *ChatSessionSnapshot) { snapshot.ActiveJob.Steps[0].Status = model.StepStatusRunning }},
		{name: "missing coding authority", change: func(snapshot *ChatSessionSnapshot) {
			var metadata map[string]any
			if err := json.Unmarshal(snapshot.ActiveJob.Job.Metadata, &metadata); err != nil {
				t.Fatal(err)
			}
			delete(metadata, "coding_execution")
			encoded, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.ActiveJob.Job.Metadata = encoded
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			snapshot := agentQuestionSnapshot(t)
			test.change(&snapshot)
			if err := validateChatSessionSnapshot(snapshot, testWorkspaceIdentity, MaxChatSessionMessages); err == nil {
				t.Fatal("contradictory agent question projection was accepted")
			}
		})
	}
}

func TestCLIChatSnapshotPreservesWaitingControlWithoutAgentQuestion(t *testing.T) {
	t.Parallel()
	snapshot := agentQuestionSnapshot(t)
	snapshot.ActiveJob.AgentPause = nil
	if err := validateChatSessionSnapshot(snapshot, testWorkspaceIdentity, MaxChatSessionMessages); err != nil {
		t.Fatalf("interrupted Agents job without a concrete question was rejected: %v", err)
	}
}
