package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPublishAIControlUpdateUsesTypedRealtimeState(t *testing.T) {
	hub := NewRealtimeHub()
	subscription, err := hub.Subscribe([]string{realtimeTopicUI}, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer subscription.Unsubscribe()
	server := &Server{realtimeHub: hub}
	state := aiControlState{
		Paused:    true,
		Counts:    map[string]int64{"pending": 3, "running": 0, "waiting_input": 0},
		UpdatedAt: time.Now().UTC(),
	}
	if err := server.publishAIControlUpdate(state, "paused"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case raw := <-subscription.Messages:
		var message realtimeMessage
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode event: %v", err)
		}
		if message.EventName != "ai-control-updated" || message.AIControl == nil || !message.AIControl.Paused {
			t.Fatalf("unexpected event: %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for AI control event")
	}
}

func TestPublishAIControlUpdateFailsWithoutRealtimeHub(t *testing.T) {
	if err := (&Server{}).publishAIControlUpdate(aiControlState{}, "resume"); err == nil {
		t.Fatal("expected unavailable realtime hub to fail")
	}
}
