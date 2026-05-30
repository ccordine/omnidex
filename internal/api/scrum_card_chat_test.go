package api

import (
	"strings"
	"testing"
	"time"
)

func TestBuildPilotChannelTimelineKeepsThinkingChronologically(t *testing.T) {
	chat := []ScrumChatMessage{
		{Role: "tool", Content: strings.Repeat("x", 3000), CreatedAt: "2026-05-29T10:00:00Z"},
		{Role: "thinking", Content: "need to inspect middleware", CreatedAt: "2026-05-29T10:00:01Z"},
		{Role: "thinking", Content: "grep login handlers next", CreatedAt: "2026-05-29T10:00:02Z"},
		{Role: "assistant", Content: "partial chunk", CreatedAt: "2026-05-29T10:00:03Z"},
		{Role: "assistant", Content: "Final agent answer about login middleware.", CreatedAt: "2026-05-29T10:00:04Z"},
		{Role: "user", Content: "are we done?", CreatedAt: "2026-05-29T10:00:05Z"},
		{Role: "assistant", Content: `{"type":"tool_call","name":"grep"}`, CreatedAt: "2026-05-29T10:00:06Z"},
		{Role: "assistant", Content: "Pilot says not yet.", CreatedAt: "2026-05-29T10:00:07Z"},
	}
	timeline := buildPilotChannelTimeline(chat)
	joined := strings.Join(func() []string {
		out := make([]string, 0, len(timeline))
		for _, msg := range timeline {
			out = append(out, msg.Role+": "+msg.Content)
		}
		return out
	}(), "\n")
	if strings.Contains(joined, strings.Repeat("x", 50)) {
		t.Fatalf("tool content leaked: %q", joined)
	}
	if !strings.Contains(joined, "inspect middleware") || !strings.Contains(joined, "grep login") {
		t.Fatalf("thinking missing or not merged: %q", joined)
	}
	if !strings.Contains(joined, "Final agent answer") {
		t.Fatalf("missing final agent reply: %q", joined)
	}
	if !strings.Contains(joined, "are we done?") {
		t.Fatalf("missing user: %q", joined)
	}
	if !strings.Contains(joined, "Pilot says not yet") {
		t.Fatalf("missing pilot reply: %q", joined)
	}
	if strings.Contains(joined, "tool_call") {
		t.Fatalf("tool-like assistant leaked: %q", joined)
	}
	if strings.Index(joined, "inspect middleware") > strings.Index(joined, "Final agent answer") {
		t.Fatalf("thinking should precede final agent reply: %q", joined)
	}
}

func TestSortScrumChatChronologicalUsesCreatedAt(t *testing.T) {
	chat := []ScrumChatMessage{
		{Role: "assistant", Content: "later", CreatedAt: "2026-05-29T12:00:00Z"},
		{Role: "user", Content: "earlier", CreatedAt: "2026-05-29T11:00:00Z"},
	}
	sorted := sortScrumChatChronological(chat)
	if len(sorted) != 2 || sorted[0].Content != "earlier" || sorted[1].Content != "later" {
		t.Fatalf("sorted=%v", sorted)
	}
}

func TestSelectRelevantPilotChunksReturnsChronologicalOrder(t *testing.T) {
	base := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	chunks := []pilotChannelChunk{
		{Role: "assistant", Text: "database migration done", CreatedAt: base, Index: 0},
		{Role: "thinking", Text: "login middleware still open", CreatedAt: base.Add(1 * time.Minute), Index: 1},
		{Role: "user", Text: "How is login middleware coming along?", CreatedAt: base.Add(2 * time.Minute), Index: 2},
		{Role: "assistant", Text: "Login middleware wired; tests pending.", CreatedAt: base.Add(3 * time.Minute), Index: 3},
	}
	s := &Server{}
	selected := s.selectRelevantPilotChunks(t.Context(), "login middleware status", chunks)
	if len(selected) < 2 {
		t.Fatalf("selected=%v", selected)
	}
	for i := 1; i < len(selected); i++ {
		if selected[i].CreatedAt.Before(selected[i-1].CreatedAt) {
			t.Fatalf("selected not chronological: %+v", selected)
		}
	}
}

func TestBuildScrumPilotChatPromptBudget(t *testing.T) {
	card := ScrumCard{
		Title:       "Test card",
		Column:      "review",
		Description: strings.Repeat("d", 5000),
		Chat: []ScrumChatMessage{
			{Role: "tool", Content: strings.Repeat("t", 5000)},
			{Role: "thinking", Content: "plan auth rollout", CreatedAt: "2026-05-29T10:00:00Z"},
			{Role: "assistant", Content: "Done with auth setup.", CreatedAt: "2026-05-29T10:00:01Z"},
			{Role: "user", Content: "What is next?", CreatedAt: "2026-05-29T10:00:02Z"},
		},
	}
	board := ScrumBoard{ProjectDirectory: "/tmp/project"}
	ctx := scrumPilotPromptContext{
		ChannelSummary: "last agent Done with auth setup.",
		ChannelFacts:   []string{"Channel timeline (chronological, caveman):", "think: plan auth rollout", "agent: Done with auth setup.", "u: What is next?"},
	}
	prompt := buildScrumPilotChatPrompt(board, card, "Steer the work", ctx)
	if len(prompt) > scrumPilotChatMaxTotalChars+200 {
		t.Fatalf("prompt too large: %d chars", len(prompt))
	}
	if !strings.Contains(prompt, "What is next?") {
		t.Fatalf("missing latest user message: %q", prompt)
	}
	if strings.Contains(prompt, strings.Repeat("t", 100)) {
		t.Fatalf("tool dump leaked into prompt")
	}
	if strings.Contains(prompt, strings.Repeat("d", 400)) {
		t.Fatalf("description not trimmed: %d chars", len(prompt))
	}
}

func TestBuildPilotCavemanContextBudget(t *testing.T) {
	chunks := make([]pilotChannelChunk, 0, 20)
	for i := 0; i < 20; i++ {
		chunks = append(chunks, pilotChannelChunk{
			Role:  "thinking",
			Text:  strings.Repeat("fact ", 40),
			Index: i,
		})
	}
	lines := buildPilotCavemanContext(chunks, scrumPilotChannelBudgetChars)
	joined := strings.Join(lines, "\n")
	if len(joined) > scrumPilotChannelBudgetChars+4 {
		t.Fatalf("caveman block too large: %d chars", len(joined))
	}
	if !strings.Contains(joined, "Channel timeline") {
		t.Fatalf("missing header: %q", joined)
	}
}

func TestHugeChannelHistoryStaysTiny(t *testing.T) {
	chat := make([]ScrumChatMessage, 0, 200)
	for i := 0; i < 150; i++ {
		chat = append(chat, ScrumChatMessage{Role: "tool", Content: strings.Repeat("noise", 500)})
		chat = append(chat, ScrumChatMessage{Role: "thinking", Content: "thinking " + strings.Repeat("z", 200)})
	}
	chat = append(chat,
		ScrumChatMessage{Role: "assistant", Content: "Implemented caching layer.", CreatedAt: "2026-05-29T10:00:00Z"},
		ScrumChatMessage{Role: "user", Content: "Did caching land?", CreatedAt: "2026-05-29T10:00:01Z"},
		ScrumChatMessage{Role: "assistant", Content: "Pilot: caching yes, deploy pending.", CreatedAt: "2026-05-29T10:00:02Z"},
	)
	card := ScrumCard{
		Title:       "Cache task",
		Description: strings.Repeat("desc ", 300),
		Chat:        chat,
	}
	board := ScrumBoard{ProjectDirectory: "/tmp/project"}
	ctx := (&Server{}).summarizeScrumPilotChannel(t.Context(), board, card, "caching deploy status", nil)
	prompt := buildScrumPilotChatPrompt(board, card, "Are we done with caching?", ctx)
	if len(prompt) > scrumPilotChatMaxTotalChars+200 {
		t.Fatalf("prompt too large after huge history: %d chars", len(prompt))
	}
	if strings.Contains(prompt, "noise") {
		t.Fatalf("tool noise leaked into prompt")
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
