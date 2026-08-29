package api

import (
	"context"
	"testing"
	"time"
)

func TestScrumRealtimeCardUpdateRequiresPostgreSQLProjection(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	server.realtimeHub = NewRealtimeHub()
	subscription, err := server.realtimeHub.Subscribe([]string{"scrum"}, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Unsubscribe()

	card := ScrumCard{ID: "card_123", Title: "Unpersisted realtime"}
	server.publishScrumCardUpdate(context.Background(), 42, card, "test refresh")

	select {
	case raw := <-subscription.Messages:
		t.Fatalf("unpersisted card produced a realtime success message: %s", raw)
	case <-time.After(25 * time.Millisecond):
	}
}
