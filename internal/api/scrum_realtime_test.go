package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestScrumModalRealtimeMessageCarriesTypedCardWithoutHTML(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	server.realtimeHub = NewRealtimeHub()
	subscription, err := server.realtimeHub.Subscribe([]string{"scrum"}, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Unsubscribe()

	card := ScrumCard{ID: "card_123", Title: "Typed realtime"}
	server.publishScrumCardUpdate(context.Background(), 42, card, "test refresh")

	select {
	case raw := <-subscription.Messages:
		if strings.Contains(string(raw), `"html"`) || strings.Contains(string(raw), "data-recyclr") {
			t.Fatalf("modal realtime must not contain HTML bundle: %s", raw)
		}
		var msg realtimeMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("invalid realtime JSON: %v", err)
		}
		if msg.EventName != "scrum-card-updated" || msg.CardID != card.ID || msg.Card == nil || msg.Card.Title != card.Title {
			t.Fatalf("unexpected realtime message: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime message")
	}
}

func TestScrumRealtimeCardPayloadIsBoundedAndOmitsConsoleLog(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	server.realtimeHub = NewRealtimeHub()
	subscription, err := server.realtimeHub.Subscribe([]string{"scrum"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Unsubscribe()
	card := ScrumCard{ID: "card_bounded", ConsoleLog: strings.Repeat("verbose output", 1_000)}
	for index := 0; index < scrumRealtimeChannelPageSize+10; index++ {
		role := "assistant"
		if index%2 == 0 {
			role = "user"
		}
		card.Chat = append(card.Chat, ScrumChatMessage{ID: fmt.Sprintf("message_%d", index), Role: role, Content: fmt.Sprintf("message %d", index)})
	}

	server.publishScrumCardUpdate(context.Background(), 1, card, "agent output")

	raw := <-subscription.Messages
	var message realtimeMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	if message.Card == nil || len(message.Card.Chat) != scrumRealtimeChannelPageSize {
		t.Fatalf("unexpected bounded card: %+v", message.Card)
	}
	if message.Card.ConsoleLog != "" || message.Card.ChatCount != scrumRealtimeChannelPageSize+10 {
		t.Fatalf("verbose card state leaked into realtime: %+v", message.Card)
	}
}
