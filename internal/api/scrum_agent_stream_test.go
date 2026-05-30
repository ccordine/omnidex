package api

import (
	"strings"
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
	line := `{"agent":"cursor","type":"message","message":"{\"type\":\"tool_call\",\"name\":\"edit\",\"status\":\"completed\",\"path\":\"src/App.tsx\"}"}`
	msgs := agentNDJSONLineToChatMessages(line)
	if len(msgs) != 1 || msgs[0].Role != "tool" {
		t.Fatalf("tool call should become activity message, got: %+v", msgs)
	}
	activity, ok := parseChannelActivity(msgs[0].Content)
	if !ok || activity.Activity != "file_change" {
		t.Fatalf("activity=%+v ok=%v", activity, ok)
	}
}

func TestAgentNDJSONLineToChatMessagesCommand(t *testing.T) {
	line := `{"agent":"codex","type":"command","message":"running tests","command":"npm test"}`
	msgs := agentNDJSONLineToChatMessages(line)
	if len(msgs) != 1 || msgs[0].Role != "tool" {
		t.Fatalf("command should become activity message, got: %+v", msgs)
	}
	activity, ok := parseChannelActivity(msgs[0].Content)
	if !ok || activity.Activity != "command" || activity.Command != "npm test" {
		t.Fatalf("activity=%+v", activity)
	}
}

func TestAgentNDJSONLineToChatMessagesFileChange(t *testing.T) {
	line := `{"agent":"codex","type":"file_change","message":"completed","files":["internal/api/foo.go"],"raw":{"changes":[{"path":"internal/api/foo.go","diff":"--- a/foo.go\\n+++ b/foo.go\\n@@ -1 +1 @@\\n-old\\n+new"}]}}`
	msgs := agentNDJSONLineToChatMessages(line)
	if len(msgs) != 1 {
		t.Fatalf("messages=%+v", msgs)
	}
	activity, ok := parseChannelActivity(msgs[0].Content)
	if !ok || activity.Activity != "file_change" || len(activity.Files) != 1 {
		t.Fatalf("activity=%+v", activity)
	}
	if !strings.Contains(activity.Diff, "@@") {
		t.Fatalf("diff=%q", activity.Diff)
	}
}

func TestAgentNDJSONLineToChatMessagesStatusDropped(t *testing.T) {
	line := `{"agent":"cursor","type":"status","message":"{\"status\":\"RUNNING\"}"}`
	msgs := agentNDJSONLineToChatMessages(line)
	if len(msgs) != 0 {
		t.Fatalf("status noise must not enter card channel, got: %+v", msgs)
	}
}

func TestAppendParsedAgentStreamLinesMergesMultilinePlainText(t *testing.T) {
	chat := appendParsedAgentStreamLines(nil, "First line of reply\nSecond line of reply\nThird line")
	if len(chat) != 1 {
		t.Fatalf("expected one merged assistant message, got %d: %+v", len(chat), chat)
	}
	if chat[0].Role != "assistant" {
		t.Fatalf("role=%q", chat[0].Role)
	}
	if !strings.Contains(chat[0].Content, "First line") || !strings.Contains(chat[0].Content, "Third line") {
		t.Fatalf("content=%q", chat[0].Content)
	}
}

func TestAppendParsedAgentStreamLinesMergesStreamingAssistant(t *testing.T) {
	line1 := `{"agent":"cursor","type":"message","message":"{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Hello\"}]}}"}`
	line2 := `{"agent":"cursor","type":"message","message":"{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Hello world\"}]}}"}`
	chat := appendParsedAgentStreamLines(nil, line1+"\n"+line2)
	if len(chat) != 1 {
		t.Fatalf("expected one assistant message, got %d: %+v", len(chat), chat)
	}
	if chat[0].Content != "Hello world" {
		t.Fatalf("content=%q", chat[0].Content)
	}
}

func TestAppendParsedAgentStreamLinesDedupes(t *testing.T) {
	chat := appendScrumChatMessage(nil, "assistant", "Working on it")
	chat = appendParsedAgentStreamLines(chat, `{"agent":"cursor","type":"status","message":"{\"status\":\"RUNNING\"}"}`)
	if len(chat) != 1 {
		t.Fatalf("expected status to be dropped, got %d messages", len(chat))
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
	foundAssistant := false
	for _, msg := range updated.Chat {
		if msg.Role == "assistant" && strings.Contains(msg.Content, "done") {
			foundAssistant = true
			break
		}
	}
	if !foundAssistant {
		t.Fatalf("expected assistant reply, got %+v", updated.Chat)
	}
}

func modelJobDetailsWithOutput(output string) model.JobDetails {
	return model.JobDetails{
		Steps: []model.Step{{Output: output, Status: "running"}},
	}
}
