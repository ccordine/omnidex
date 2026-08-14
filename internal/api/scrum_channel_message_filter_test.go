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
	messages := []ScrumChatMessage{{
		ID: "assistant-content", Role: "assistant", Content: content,
		CreatedAt: "2026-08-13T12:00:00Z",
	}}
	appends, err := scrumChannelMessageAppends(messages)
	if err != nil {
		t.Fatal(err)
	}
	if len(appends) != 1 {
		t.Fatalf("assistant content was hidden by content inference: %+v", appends)
	}
	if appends[0].Role != "assistant" || appends[0].Content != content {
		t.Fatalf("assistant content changed transport role: %+v", appends[0])
	}
}

func TestMarkerLikeUserAssistantAndSystemContentIsPreserved(t *testing.T) {
	content := " \n[[agent-stream-len:42]]\n[[context-sync:7]]\nJob status: complete\n "
	for _, role := range []string{"user", "assistant", "system"} {
		t.Run(role, func(t *testing.T) {
			appends, err := scrumChannelMessageAppends([]ScrumChatMessage{{
				ID: "marker-content-" + role, Role: role, Content: content,
				CreatedAt: "2026-08-13T12:00:00Z",
			}})
			if err != nil {
				t.Fatal(err)
			}
			if len(appends) != 1 || appends[0].Role != role || appends[0].Content != content {
				t.Fatalf("%s content was hidden, reclassified, or rewritten: %+v", role, appends)
			}
		})
	}
}
