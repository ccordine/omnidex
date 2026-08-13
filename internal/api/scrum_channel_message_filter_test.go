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
	messages, err := displayScrumChannelMessages(ScrumCard{Chat: []ScrumChatMessage{{Role: "assistant", Content: content}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("assistant content was hidden by content inference: %+v", messages)
	}
	if messages[0].Role != "assistant" || messages[0].Content != content {
		t.Fatalf("assistant content changed transport role: %+v", messages[0])
	}
}

func TestMarkerLikeUserAssistantAndSystemContentIsPreserved(t *testing.T) {
	content := " \n[[agent-stream-len:42]]\n[[context-sync:7]]\nJob status: complete\n "
	for _, role := range []string{"user", "assistant", "system"} {
		t.Run(role, func(t *testing.T) {
			messages, err := displayScrumChannelMessages(ScrumCard{Chat: []ScrumChatMessage{{Role: role, Content: content}}})
			if err != nil {
				t.Fatal(err)
			}
			if len(messages) != 1 || messages[0].Role != role || messages[0].Content != content {
				t.Fatalf("%s content was hidden, reclassified, or rewritten: %+v", role, messages)
			}
		})
	}
}
