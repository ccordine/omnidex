package api

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/agentstream"
	"github.com/gryph/omnidex/internal/model"
)

func TestDisplayScrumChannelMessagesPreservesTypedContentVerbatim(t *testing.T) {
	card := ScrumCard{
		Chat: []ScrumChatMessage{
			{Role: "user", Content: "fix it", CreatedAt: "2026-05-29T10:00:00Z"},
			{Role: "assistant", Content: "Here is the fix.", CreatedAt: "2026-05-29T10:00:01Z"},
			{Role: "assistant", Content: "External agent session completed", CreatedAt: "2026-05-29T10:00:02Z"},
			{Role: "system", Content: "Agent finished", CreatedAt: "2026-05-29T10:00:03Z"},
		},
	}
	messages, err := displayScrumChannelMessages(card)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 {
		t.Fatalf("expected every typed message, got %+v", messages)
	}
	if messages[2].Content != "External agent session completed" || messages[3].Content != "Agent finished" {
		t.Fatalf("message content was inferred as hidden noise: %+v", messages)
	}
}

func TestDisplayScrumChannelMessagesShowsToolActivity(t *testing.T) {
	card := ScrumCard{
		Chat: []ScrumChatMessage{
			{Role: "user", Content: "fix auth", CreatedAt: "2026-05-29T10:00:00Z"},
			{Role: "tool", Content: formatChannelActivity(ChannelActivity{Activity: "command", Title: "npm test", Command: "npm test", Status: "running"}), CreatedAt: "2026-05-29T10:00:01Z"},
			{Role: "status", Content: "Agent running…", CreatedAt: "2026-05-29T10:00:02Z"},
			{Role: "thinking", Content: "checking middleware", CreatedAt: "2026-05-29T10:00:03Z"},
			{Role: "tool", Content: formatChannelActivity(ChannelActivity{Activity: "file_change", Title: "src/auth.go", Files: []string{"src/auth.go"}, Status: "completed"}), CreatedAt: "2026-05-29T10:00:04Z"},
			{Role: "assistant", Content: "Auth middleware wired.", CreatedAt: "2026-05-29T10:00:05Z"},
		},
	}
	messages, err := displayScrumChannelMessages(card)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 6 {
		t.Fatalf("expected every typed message, got %+v", messages)
	}
}

func TestDisplayScrumChannelMessagesSortedByTime(t *testing.T) {
	card := ScrumCard{
		Chat: []ScrumChatMessage{
			{Role: "assistant", Content: "second", CreatedAt: "2026-05-29T12:00:00Z"},
			{Role: "thinking", Content: "first thought", CreatedAt: "2026-05-29T11:00:00Z"},
			{Role: "user", Content: "zeroth", CreatedAt: "2026-05-29T10:00:00Z"},
		},
	}
	messages, err := displayScrumChannelMessages(card)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages=%v", messages)
	}
	if messages[0].Content != "zeroth" || messages[1].Content != "first thought" || messages[2].Content != "second" {
		t.Fatalf("messages out of order: %+v", messages)
	}
}

func TestSortScrumChatChronologicalPreservesIndexWhenTimesMissing(t *testing.T) {
	chat := []ScrumChatMessage{
		{Role: "user", Content: "b"},
		{Role: "assistant", Content: "a"},
	}
	sorted := sortScrumChatChronological(chat)
	if sorted[0].Content != "b" || sorted[1].Content != "a" {
		t.Fatalf("sorted=%v", sorted)
	}
}

func TestSortScrumChatChronologicalParsesNanoTimestamps(t *testing.T) {
	chat := []ScrumChatMessage{
		{Role: "assistant", Content: "later", CreatedAt: time.Date(2026, 5, 29, 12, 0, 0, 500, time.UTC).Format(time.RFC3339Nano)},
		{Role: "user", Content: "earlier", CreatedAt: time.Date(2026, 5, 29, 12, 0, 0, 100, time.UTC).Format(time.RFC3339Nano)},
	}
	sorted := sortScrumChatChronological(chat)
	if sorted[0].Content != "earlier" {
		t.Fatalf("sorted=%v", sorted)
	}
}

func TestSyncRunningJobChannelChatIncremental(t *testing.T) {
	lineOne := scrumAgentEventLine(t, agentstream.EventMessage, "line one")
	lineTwo := scrumAgentEventLine(t, agentstream.EventMessage, "line two")
	job := model.JobDetails{
		Job:   model.Job{ID: 3, Status: model.JobStatusRunning},
		Steps: []model.Step{{Action: "external_agent_execute", Output: lineOne + "\n" + lineTwo + "\n"}},
	}
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{Chat: []ScrumChatMessage{{Role: "system", Content: "Job #1 queued"}}})

	updated, ok, err := syncRunningJobChannelChat(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected first sync")
	}
	if len(updated.Chat) < 2 {
		t.Fatalf("chat len=%d", len(updated.Chat))
	}
	if !strings.Contains(updated.Chat[1].Content, "line one") {
		t.Fatalf("assistant=%q", updated.Chat[1].Content)
	}

	updated2, ok, err := syncRunningJobChannelChat(updated, job)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no duplicate sync")
	}
	foundLineTwo := false
	for _, msg := range updated2.Chat {
		if strings.Contains(msg.Content, "line two") {
			foundLineTwo = true
		}
	}
	if !foundLineTwo {
		t.Fatalf("chat=%v", updated2.Chat)
	}
}

func TestDisplayScrumChannelMessagesHydratesLegacyConsole(t *testing.T) {
	card := ScrumCard{
		ConsoleLog: "queued for play\nagent stream:\nhello world",
	}
	messages, err := displayScrumChannelMessages(card)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) == 0 {
		t.Fatal("expected hydrated messages")
	}
	found := false
	for _, msg := range messages {
		if strings.Contains(msg.Content, "hello world") {
			found = true
		}
	}
	if !found {
		t.Fatalf("messages=%v", messages)
	}
}

func TestDisplayScrumChannelMessagesRejectsUnknownStoredRole(t *testing.T) {
	_, err := displayScrumChannelMessages(ScrumCard{Chat: []ScrumChatMessage{{
		ID: "unknown-role", Role: "persona", Content: "do not coerce me",
	}}})
	if err == nil || !strings.Contains(err.Error(), `unsupported role "persona"`) {
		t.Fatalf("error=%v, want loud unsupported role failure", err)
	}
}

func TestAppendScrumChannelEventWritesChatAndConsole(t *testing.T) {
	card := appendScrumChannelEvent(ScrumCard{}, "system", "Queued for play")
	if len(card.Chat) != 1 || card.Chat[0].Role != "system" {
		t.Fatalf("chat=%v", card.Chat)
	}
	if !strings.Contains(card.ConsoleLog, "Queued for play") {
		t.Fatalf("console=%q", card.ConsoleLog)
	}
}
