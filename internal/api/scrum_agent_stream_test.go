package api

import (
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestAgentNDJSONLineToChatMessagesAssistant(t *testing.T) {
	line := `{"agent":"cursor","type":"message","message":"{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Hello world\"}]}}"}`
	msgs := agentNDJSONLineToChatMessages(line)
	if len(msgs) != 1 || msgs[0].Role != "assistant" || msgs[0].Content != "Hello world" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
}

func TestAgentNDJSONLineToChatMessagesToolCall(t *testing.T) {
	line := `{"agent":"cursor","type":"message","message":"{\"type\":\"tool_call\",\"name\":\"edit\",\"status\":\"completed\"}"}`
	msgs := agentNDJSONLineToChatMessages(line)
	if len(msgs) != 1 || msgs[0].Role != "tool" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
}

func TestAppendParsedAgentStreamLinesDedupes(t *testing.T) {
	chat := appendScrumChatMessage(nil, "status", "Agent running…")
	chat = appendParsedAgentStreamLines(chat, `{"agent":"cursor","type":"status","message":"{\"status\":\"RUNNING\"}"}`)
	if len(chat) != 1 {
		t.Fatalf("expected duplicate status to be skipped, got %d messages", len(chat))
	}
}

func TestSyncRunningJobChannelChatParsesEvents(t *testing.T) {
	card := ScrumCard{}
	job := modelJobDetailsWithOutput(`{"agent":"cursor","type":"started","message":"started"}` + "\n" +
		`{"agent":"cursor","type":"message","message":"{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"done\"}]}}"}`)
	updated, ok := syncRunningJobChannelChat(card, job)
	if !ok {
		t.Fatal("expected sync")
	}
	if len(updated.Chat) < 2 {
		t.Fatalf("expected parsed chat messages, got %+v", updated.Chat)
	}
}

func modelJobDetailsWithOutput(output string) model.JobDetails {
	return model.JobDetails{
		Steps: []model.Step{{Output: output, Status: "running"}},
	}
}
