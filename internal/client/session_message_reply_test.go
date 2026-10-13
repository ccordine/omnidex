package client

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestCLIActiveTranscriptKeepsOriginalTaskBindingWithConversationReplies(t *testing.T) {
	job := model.Job{ID: 41, Status: model.JobStatusRunning, Instruction: "Apply the requested behavior."}
	messages := []model.ChannelMessage{
		{ID: 1, Role: model.ChannelMessageRoleUser, Content: job.Instruction, Turn: &model.ChannelMessageTurnState{JobID: job.ID, Status: job.Status}},
		{ID: 2, Role: model.ChannelMessageRoleAssistant, Content: "The task is still running."},
	}
	if err := validateChatSessionActiveTurn(messages, &job, 1); err != nil {
		t.Fatalf("typed conversation reply displaced task binding: %v", err)
	}
	messages = append(messages, model.ChannelMessage{ID: 3, Role: model.ChannelMessageRoleUser, Content: "A different task."})
	if err := validateChatSessionActiveTurn(messages, &job, 1); err == nil {
		t.Fatal("another user task bypassed active binding")
	}
	messages = messages[:2]
	messages[0].Content = "A different original task."
	if err := validateChatSessionActiveTurn(messages, &job, 1); err == nil {
		t.Fatal("reply weakened exact original task binding")
	}
}
