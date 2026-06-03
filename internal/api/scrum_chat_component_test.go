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

func TestScrumCardChatComponentSummarizesActivityJSON(t *testing.T) {
	card := ScrumCard{
		ID: "card-1",
		Chat: []ScrumChatMessage{
			{Role: "tool", Content: `{"activity":"command","title":"npm test","status":"completed","command":"npm test","detail":"all tests passed"}`, CreatedAt: "2026-06-03T12:00:00Z"},
		},
	}

	html := renderScrumCardChatHTML(card)
	if !strings.Contains(html, "npm test") || !strings.Contains(html, "Details") {
		t.Fatalf("activity summary missing: %s", html)
	}
	if !strings.Contains(html, `data-chat-message-detail-card`) || !strings.Contains(html, `template data-chat-message-detail`) {
		t.Fatalf("activity detail template missing: %s", html)
	}
	if strings.Contains(html, `{&quot;activity&quot;`) {
		t.Fatalf("raw activity json leaked into visible html: %s", html)
	}
}
