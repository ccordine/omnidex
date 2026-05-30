package api

import (
	"strings"
	"testing"
)

func TestMeasureScrumPilotRawContextIncludesFullChat(t *testing.T) {
	card := ScrumCard{
		Title:       "Auth",
		Description: strings.Repeat("d", 1000),
		Chat: []ScrumChatMessage{
			{Role: "tool", Content: strings.Repeat("t", 5000)},
			{Role: "thinking", Content: "plan"},
			{Role: "user", Content: "go"},
		},
	}
	board := ScrumBoard{ProjectDirectory: "/tmp/project"}
	report := measureScrumPilotRawContext(board, card, "steer please")
	if report.RawChars < 6000 {
		t.Fatalf("raw too small: %d", report.RawChars)
	}
	if report.ToolMessages != 1 || report.ThinkingMessages != 1 {
		t.Fatalf("report=%+v", report)
	}
	if report.ChatMessages != 4 {
		t.Fatalf("chat messages=%d", report.ChatMessages)
	}
}

func TestHugeHistoryShrinkTelemetrySizes(t *testing.T) {
	chat := make([]ScrumChatMessage, 0, 200)
	for i := 0; i < 150; i++ {
		chat = append(chat, ScrumChatMessage{Role: "tool", Content: strings.Repeat("noise", 500)})
		chat = append(chat, ScrumChatMessage{Role: "thinking", Content: "thinking " + strings.Repeat("z", 200)})
	}
	chat = append(chat,
		ScrumChatMessage{Role: "assistant", Content: "Implemented caching layer.", CreatedAt: "2026-05-29T10:00:00Z"},
		ScrumChatMessage{Role: "user", Content: "Did caching land?", CreatedAt: "2026-05-29T10:00:01Z"},
	)
	card := ScrumCard{Title: "Cache task", Description: strings.Repeat("desc ", 300), Chat: chat}
	board := ScrumBoard{ProjectDirectory: "/tmp/project"}
	raw := measureScrumPilotRawContext(board, card, "Are we done with caching?")
	ctx := (&Server{}).summarizeScrumPilotChannel(t.Context(), board, card, "caching deploy status", nil)
	shrunk := buildScrumPilotChatPrompt(board, card, "Are we done with caching?", ctx)
	if raw.RawChars < 300000 {
		t.Fatalf("expected huge raw context, got %d", raw.RawChars)
	}
	if len(shrunk) > 1200 {
		t.Fatalf("expected tiny shrunk prompt, got %d", len(shrunk))
	}
}
