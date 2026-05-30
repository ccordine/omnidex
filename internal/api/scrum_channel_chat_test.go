package api

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestSyncRunningJobChannelChatIncremental(t *testing.T) {
	card := ScrumCard{Chat: []ScrumChatMessage{{Role: "system", Content: "Job #1 queued"}}}
	job := model.JobDetails{
		Steps: []model.Step{{Output: "line one\nline two"}},
	}

	updated, ok := syncRunningJobChannelChat(card, job)
	if !ok {
		t.Fatal("expected first sync")
	}
	if len(updated.Chat) != 2 {
		t.Fatalf("chat len=%d", len(updated.Chat))
	}
	if !strings.Contains(updated.Chat[1].Content, "agent stream:") || !strings.Contains(updated.Chat[1].Content, "line one") {
		t.Fatalf("assistant=%q", updated.Chat[1].Content)
	}

	updated2, ok := syncRunningJobChannelChat(updated, job)
	if ok {
		t.Fatal("expected no duplicate sync")
	}
	if !strings.Contains(updated2.Chat[1].Content, "line two") {
		t.Fatalf("assistant=%q", updated2.Chat[1].Content)
	}
}

func TestDisplayScrumChannelMessagesHydratesLegacyConsole(t *testing.T) {
	card := ScrumCard{
		ConsoleLog: "queued for play\nagent stream:\nhello world",
	}
	messages := displayScrumChannelMessages(card)
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

func TestAppendScrumChannelEventWritesChatAndConsole(t *testing.T) {
	card := appendScrumChannelEvent(ScrumCard{}, "system", "Queued for play")
	if len(card.Chat) != 1 || card.Chat[0].Role != "system" {
		t.Fatalf("chat=%v", card.Chat)
	}
	if !strings.Contains(card.ConsoleLog, "Queued for play") {
		t.Fatalf("console=%q", card.ConsoleLog)
	}
}
