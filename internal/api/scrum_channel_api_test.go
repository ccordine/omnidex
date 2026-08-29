package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestScrumChannelPageIsTypedPaginatedAndContainsNoHTML(t *testing.T) {
	card := ScrumCard{ID: "card-1", Chat: make([]ScrumChatMessage, 0, 55)}
	for index := 0; index < 55; index++ {
		role := "assistant"
		if index%2 == 0 {
			role = "user"
		}
		card.Chat = append(card.Chat, ScrumChatMessage{
			ID: fmt.Sprintf("message_%02d", index), Role: role, Content: fmt.Sprintf("message %02d", index),
		})
	}
	beforeCursor, err := encodeScrumChannelCursor(5, true)
	if err != nil {
		t.Fatal(err)
	}
	page := scrumChannelMessagePage{
		Messages: card.Chat[5:], BeforeCursor: beforeCursor,
		HasMore: true, Total: int64(len(card.Chat)),
	}
	body, err := json.Marshal(map[string]any{
		"messages":      page.Messages,
		"before_cursor": page.BeforeCursor,
		"has_more":      page.HasMore,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"html"`) || strings.Contains(string(body), "data-recyclr") {
		t.Fatalf("typed channel page contains legacy HTML: %s", body)
	}
	if len(page.Messages) != 50 || !page.HasMore || page.BeforeCursor == "" {
		t.Fatalf("unexpected page: count=%d has_more=%v before=%q", len(page.Messages), page.HasMore, page.BeforeCursor)
	}
}

func TestScrumChannelCursorRejectsNoncanonicalAliases(t *testing.T) {
	for _, raw := range []string{
		" scrumchat_v1_1", "scrumchat_v1_1 ", "scrumchat_v1_01", "scrumchat_v1_+1", "scrumchat_v1_A",
	} {
		if _, err := parseScrumChannelCursor(raw); err == nil {
			t.Fatalf("noncanonical Scrum channel cursor accepted: %q", raw)
		}
	}
	if got, err := parseScrumChannelCursor("scrumchat_v1_a"); err != nil || got != 10 {
		t.Fatalf("canonical cursor got=%d error=%v", got, err)
	}
	maximum, err := encodeScrumChannelCursor(maxScrumChannelCursorOrdinal, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := parseScrumChannelCursor(maximum); err != nil || got != maxScrumChannelCursorOrdinal {
		t.Fatalf("maximum cursor got=%d error=%v", got, err)
	}
	oneOver := "scrumchat_v1_" + strconv.FormatInt(maxScrumChannelCursorOrdinal+1, 36)
	if _, err := parseScrumChannelCursor(oneOver); err == nil {
		t.Fatal("one-over exact cursor authority was accepted")
	}
	if _, err := encodeScrumChannelCursor(maxScrumChannelCursorOrdinal+1, true); err == nil {
		t.Fatal("one-over cursor encoding did not fail loudly")
	}
}

func TestScrumChannelHasNoParallelInMemoryImmutableProjection(t *testing.T) {
	source, err := os.ReadFile("scrum_channel_page.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "scrumChannelImmutableResultProjection") {
		t.Fatal("Scrum channel retained a parallel in-memory immutable result projection")
	}
}
