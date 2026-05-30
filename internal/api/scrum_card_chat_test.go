package api

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/omni"
)

func TestSelectScrumPilotChatHistorySkipsAgentStreamMarker(t *testing.T) {
	chat := []ScrumChatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "partial\n[[agent-stream-len:9999]]\n"},
		{Role: "tool", Content: strings.Repeat("x", 2000)},
		{Role: "user", Content: "follow up"},
	}
	lines := selectScrumPilotChatHistory(chat)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "[[agent-stream-len:") {
		t.Fatalf("agent stream marker leaked: %q", joined)
	}
	if !strings.Contains(joined, "follow up") {
		t.Fatalf("missing follow up: %q", joined)
	}
}

func TestBuildScrumPilotChatPromptBudget(t *testing.T) {
	card := ScrumCard{
		Title:       "Test card",
		Column:      "review",
		Description: strings.Repeat("d", 5000),
		Chat: []ScrumChatMessage{
			{Role: "assistant", Content: strings.Repeat("a", 5000)},
			{Role: "user", Content: "What is next?"},
		},
	}
	board := ScrumBoard{ProjectDirectory: "/tmp/project"}
	prompt := buildScrumPilotChatPrompt(board, card, "Steer the work", scrumPilotPromptContext{})
	if len(prompt) > scrumPilotChatMaxTotalChars+200 {
		t.Fatalf("prompt too large: %d chars", len(prompt))
	}
	if !strings.Contains(prompt, "What is next?") {
		t.Fatalf("missing latest user message: %q", prompt)
	}
}

func TestFormatScrumMinimalContextForPilot(t *testing.T) {
	text := formatScrumMinimalContextForPilot(omni.MinimalContext{
		Summary:     "Auth flow half done.",
		Facts:       []string{"login.go touched", "tests red"},
		Constraints: []string{"stay in /tmp/project"},
		OpenItems:   []string{"fix session cookie"},
	})
	if !strings.Contains(text, "Auth flow half done.") {
		t.Fatalf("missing summary: %q", text)
	}
	if !strings.Contains(text, "login.go touched") {
		t.Fatalf("missing facts: %q", text)
	}
}

func TestScrumChannelTranscriptForSummarizerSkipsAgentStream(t *testing.T) {
	chat := []ScrumChatMessage{
		{Role: "assistant", Content: "partial\n[[agent-stream-len:9999]]\n"},
		{Role: "user", Content: "keep me"},
	}
	transcript := scrumChannelTranscriptForSummarizer(chat)
	if strings.Contains(transcript, "[[agent-stream-len:") {
		t.Fatalf("agent stream leaked: %q", transcript)
	}
	if !strings.Contains(transcript, "keep me") {
		t.Fatalf("missing user turn: %q", transcript)
	}
}

func TestBuildScrumPilotChatPromptUsesMinifiedContext(t *testing.T) {
	card := ScrumCard{
		Title: "Big card",
		Chat: []ScrumChatMessage{
			{Role: "assistant", Content: strings.Repeat("a", 5000)},
		},
	}
	board := ScrumBoard{ProjectDirectory: "/tmp/project"}
	ctx := scrumPilotPromptContext{
		ChannelSummary: "Channel context (minified):\nSummary: work in progress",
		MemoryLines:    []string{"prior decision: use sqlite"},
		RecentTurns:    []string{"Recent user/assistant turns:", "user: hi"},
	}
	prompt := buildScrumPilotChatPrompt(board, card, "next?", ctx)
	if !strings.Contains(prompt, "work in progress") {
		t.Fatalf("missing minified summary: %q", prompt)
	}
	if !strings.Contains(prompt, "prior decision: use sqlite") {
		t.Fatalf("missing memory: %q", prompt)
	}
	if strings.Contains(prompt, strings.Repeat("a", 100)) {
		t.Fatalf("raw history leaked into prompt")
	}
}

func TestFormatScrumPilotChatErrorContextExceeded(t *testing.T) {
	msg := formatScrumPilotChatError(errString("model error: context length exceeded"))
	if !strings.Contains(strings.ToLower(msg), "context limit") {
		t.Fatalf("msg=%q", msg)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
