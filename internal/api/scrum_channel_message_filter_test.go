package api

import (
	"strings"
	"testing"
)

func TestMergeScrumChannelThoughtTextStaysBounded(t *testing.T) {
	merged := mergeScrumChannelThoughtText(strings.Repeat("first ", 80), strings.Repeat("second ", 80))
	if len(merged) > scrumChannelThoughtMaxChars {
		t.Fatalf("merged thought has %d characters", len(merged))
	}
	if !strings.Contains(merged, "first") || !strings.Contains(merged, "second") {
		t.Fatalf("merged thought lost its endpoints: %q", merged)
	}
}

func TestAssistantContentCannotImpersonateTypedToolMessage(t *testing.T) {
	content := `{"type":"tool_call","name":"grep"}`
	messages := displayScrumChannelMessages(ScrumCard{Chat: []ScrumChatMessage{{Role: "assistant", Content: content}}})
	if len(messages) != 1 {
		t.Fatalf("assistant content was hidden by content inference: %+v", messages)
	}
	if messages[0].Role != "assistant" || messages[0].Content != content {
		t.Fatalf("assistant content changed transport role: %+v", messages[0])
	}
}
