package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestScrumCardActionProjectionRemovesGrowingHistories(t *testing.T) {
	t.Parallel()
	card := ScrumCard{
		ID: "card-1", Title: "Authoritative", Description: "retained", Column: "review",
		Checklist: []ScrumChecklistItem{{ID: "item-1", Text: "kept", Done: true}},
		RefFiles:  []string{"docs/kept.md"}, Tags: []string{"typed"},
		TestCriteria: []ScrumChecklistItem{{ID: "test-1", Text: "passes"}},
		Chat: []ScrumChatMessage{
			{ID: "message-1", Role: "user", Content: "FORBIDDEN_CHAT_BODY"},
			{ID: "message-2", Role: "assistant", Content: "FORBIDDEN_CHAT_BODY_2"},
		},
		PendingChannelMessages: []ScrumChatMessage{
			{ID: "pending-1", Role: "assistant", Content: "FORBIDDEN_PENDING_BODY"},
		},
		ChatCount: 2,
		UpdatedAt: "2026-08-13T12:00:00Z",
	}

	projected, err := scrumCardActionProjection(card)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Chat) != 0 || projected.PendingChannelMessages != nil {
		t.Fatalf("growing history survived action projection: %+v", projected)
	}
	if projected.ChatCount != 2 {
		t.Fatalf("history count=%d want 2", projected.ChatCount)
	}
	if projected.Title != card.Title || len(projected.Checklist) != 1 || len(projected.RefFiles) != 1 || len(projected.TestCriteria) != 1 {
		t.Fatalf("bounded mutation fields changed: %+v", projected)
	}
	raw, err := json.Marshal(map[string]any{"card": projected})
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{"FORBIDDEN_CHAT_BODY", "FORBIDDEN_PENDING_BODY"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("history marker %q leaked: %s", forbidden, serialized)
		}
	}
	for _, required := range []string{`"chat":[]`, `"chat_count":2`} {
		if !strings.Contains(serialized, required) {
			t.Fatalf("bounded response missing %s: %s", required, serialized)
		}
	}
}

func TestScrumCardActionProjectionRejectsNonAuthoritativeInputs(t *testing.T) {
	t.Parallel()
	for name, card := range map[string]ScrumCard{
		"missing id":    {Title: "no identity"},
		"board summary": {ID: "card-1", Summary: true},
	} {
		name, card := name, card
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := scrumCardActionProjection(card); err == nil {
				t.Fatal("expected explicit projection failure")
			}
		})
	}
}

func TestScrumCardActionHandlersCannotWriteUnprojectedCards(t *testing.T) {
	t.Parallel()
	paths := map[string]int{
		"scrum_handlers.go":                 2,
		"scrum_card_state_handlers.go":      1,
		"scrum_card_file_upload_handler.go": 0,
		"scrum_card_ticket.go":              1,
		"scrum_card_item_handler.go":        1,
		"scrum_play_queue.go":               2,
		"project_play.go":                   3,
	}
	for path, expectedProjections := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		if count := strings.Count(text, "scrumCardActionProjection("); count != expectedProjections {
			t.Errorf("%s action projections=%d want %d", path, count, expectedProjections)
		}
		for _, forbidden := range []string{
			`map[string]any{"card": card}`,
			`map[string]any{"card": updated`,
			`map[string]any{"card": result}`,
			`"card":       card,`,
			`"card":    updated,`,
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s retains unprojected card response %q", path, forbidden)
			}
		}
	}
	channelSource, err := os.ReadFile("scrum_channel_operation.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(channelSource), "card.Chat = make([]ScrumChatMessage, 0, len(result.Messages))") ||
		!strings.Contains(string(channelSource), "card.ChannelBeforeCursor, err = encodeScrumChannelCursor") ||
		strings.Contains(string(channelSource), "scrumCardActionProjection(") {
		t.Fatal("channel replay must retain only its separate bounded immutable projection")
	}
}
