package api

import (
	"strings"
	"testing"
)

func TestScrumCardChatComponentRendersStableMessageIDs(t *testing.T) {
	card := ScrumCard{
		ID: "card-1",
		Chat: []ScrumChatMessage{
			{Role: "user", Content: "ship it", CreatedAt: "2026-06-03T12:00:00Z"},
			{ID: "known-id", Role: "assistant", Content: "done", CreatedAt: "2026-06-03T12:01:00Z"},
		},
	}

	html := renderScrumCardChatHTML(card)
	if !strings.Contains(html, `data-recyclr-sink="chat-message-chatmsg_`) {
		t.Fatalf("legacy message did not get deterministic chat message sink: %s", html)
	}
	if !strings.Contains(html, `data-recyclr-sink="chat-message-known-id"`) {
		t.Fatalf("existing message id was not preserved: %s", html)
	}
	if strings.Contains(html, "message-pending") {
		t.Fatalf("idle card rendered working message: %s", html)
	}
}

func TestScrumCardChatComponentRendersWorkingMessageWhenBusy(t *testing.T) {
	card := ScrumCard{
		ID:        "card-1",
		PlayState: "running",
		Chat: []ScrumChatMessage{
			{Role: "user", Content: "continue", CreatedAt: "2026-06-03T12:00:00Z"},
		},
	}

	html := renderScrumCardChatHTML(card)
	if !strings.Contains(html, `data-chat-component-working-message`) {
		t.Fatalf("busy card did not render working message: %s", html)
	}
	if !scrumCardChatBusy(card) {
		t.Fatalf("running card should be busy")
	}
}
