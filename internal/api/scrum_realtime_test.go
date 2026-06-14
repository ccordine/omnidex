package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestScrumModalRealtimeMessageCarriesTypedCardWithoutHTML(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	server.realtimeHub = NewRealtimeHub()
	_, outbound, unsubscribe, err := server.realtimeHub.Subscribe([]string{"scrum"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsubscribe()

	card := ScrumCard{ID: "card_123", Title: "Typed realtime"}
	server.publishScrumModalCardRefresh(context.Background(), 42, card, "test refresh")

	select {
	case raw := <-outbound:
		if strings.Contains(string(raw), `"html"`) || strings.Contains(string(raw), "data-recyclr") {
			t.Fatalf("modal realtime must not contain HTML bundle: %s", raw)
		}
		var msg realtimeMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("invalid realtime JSON: %v", err)
		}
		if msg.EventName != "scrum-card-modal-refresh" || msg.CardID != card.ID || msg.Card == nil || msg.Card.Title != card.Title {
			t.Fatalf("unexpected realtime message: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime message")
	}
}
