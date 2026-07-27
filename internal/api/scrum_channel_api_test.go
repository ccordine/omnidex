package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestScrumChannelPageIsTypedPaginatedAndContainsNoHTML(t *testing.T) {
	card := ScrumCard{ID: "card_1", Title: "Paginated channel"}
	for index := 0; index < 55; index++ {
		role := "assistant"
		if index%2 == 0 {
			role = "user"
		}
		card.Chat = appendScrumChatMessage(card.Chat, role, fmt.Sprintf("message %02d", index))
	}

	page, err := scrumChannelMessagePageFor(card, 50, "")
	if err != nil {
		t.Fatal(err)
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
