package api

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRealtimeHubRejectsClientsOverLimit(t *testing.T) {
	hub := NewRealtimeHub(RealtimeHubOptions{MaxClients: 1})

	first, err := hub.Subscribe([]string{"ui"}, 0)
	if err != nil {
		t.Fatalf("first Subscribe() error: %v", err)
	}
	defer first.Unsubscribe()

	_, err = hub.Subscribe([]string{"ui"}, 0)
	if !errors.Is(err, ErrRealtimeHubFull) {
		t.Fatalf("second Subscribe() error=%v want ErrRealtimeHubFull", err)
	}
}

func TestServerRejectsMissingRealtimeHubInsteadOfCreatingFallback(t *testing.T) {
	server := &Server{}
	if _, err := server.requireRealtimeHub(); !errors.Is(err, ErrRealtimeHubUnavailable) {
		t.Fatalf("requireRealtimeHub() error=%v want ErrRealtimeHubUnavailable", err)
	}
	if server.realtimeHub != nil {
		t.Fatal("missing realtime hub must not create a hidden fallback hub")
	}
}

func TestRealtimeHubUnsubscribeFreesCapacity(t *testing.T) {
	hub := NewRealtimeHub(RealtimeHubOptions{MaxClients: 1})

	first, err := hub.Subscribe([]string{"ui"}, 0)
	if err != nil {
		t.Fatalf("first Subscribe() error: %v", err)
	}
	first.Unsubscribe()

	second, err := hub.Subscribe([]string{"ui"}, 0)
	if err != nil {
		t.Fatalf("second Subscribe() error: %v", err)
	}
	second.Unsubscribe()
}

func TestRealtimeHubReplaysMessagesAfterLastSeenID(t *testing.T) {
	hub := NewRealtimeHub(RealtimeHubOptions{ReplayCapacity: 8, ClientBuffer: 2})
	first, err := hub.Broadcast([]string{"scrum"}, realtimeMessage{EventName: "scrum-card-updated", StateKey: "card:1", Reason: "first"})
	if err != nil {
		t.Fatalf("first Broadcast: %v", err)
	}
	second, err := hub.Broadcast([]string{"scrum"}, realtimeMessage{EventName: "scrum-card-updated", StateKey: "card:1", Reason: "second"})
	if err != nil {
		t.Fatalf("second Broadcast: %v", err)
	}

	subscription, err := hub.Subscribe([]string{"scrum"}, first.MessageID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Unsubscribe()
	if subscription.ReplayGap {
		t.Fatal("unexpected replay gap")
	}
	if subscription.ReplayCount != 1 {
		t.Fatalf("replay count=%d want 1", subscription.ReplayCount)
	}
	select {
	case raw := <-subscription.Messages:
		if got := realtimePayloadID(raw); got != second.MessageID {
			t.Fatalf("replayed id=%d want %d", got, second.MessageID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replay")
	}
}

func TestRealtimeHubDisconnectsSlowClientAndRetainsReplay(t *testing.T) {
	hub := NewRealtimeHub(RealtimeHubOptions{ReplayCapacity: 8, ClientBuffer: 1})
	slow, err := hub.Subscribe([]string{"scrum"}, 0)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	first, err := hub.Broadcast([]string{"scrum"}, realtimeMessage{EventName: "scrum-card-updated", StateKey: "card:1", Reason: "first"})
	if err != nil {
		t.Fatalf("first Broadcast: %v", err)
	}
	second, err := hub.Broadcast([]string{"scrum"}, realtimeMessage{EventName: "scrum-card-updated", StateKey: "card:1", Reason: "second"})
	if err != nil {
		t.Fatalf("second Broadcast: %v", err)
	}
	if second.DisconnectedClients != 1 {
		t.Fatalf("disconnected clients=%d want 1", second.DisconnectedClients)
	}
	if got := realtimePayloadID(<-slow.Messages); got != first.MessageID {
		t.Fatalf("buffered id=%d want %d", got, first.MessageID)
	}
	if _, ok := <-slow.Messages; ok {
		t.Fatal("slow client channel must close explicitly")
	}

	reconnected, err := hub.Subscribe([]string{"scrum"}, first.MessageID)
	if err != nil {
		t.Fatalf("reconnect Subscribe: %v", err)
	}
	defer reconnected.Unsubscribe()
	if got := realtimePayloadID(<-reconnected.Messages); got != second.MessageID {
		t.Fatalf("replayed id=%d want %d", got, second.MessageID)
	}
}

func TestRealtimeHubReportsReplayGap(t *testing.T) {
	hub := NewRealtimeHub(RealtimeHubOptions{ReplayCapacity: 2, ClientBuffer: 2})
	for index := 0; index < 4; index++ {
		if _, err := hub.Broadcast([]string{"ui"}, realtimeMessage{EventName: "state", StateKey: "state", Reason: string(rune('a' + index))}); err != nil {
			t.Fatalf("Broadcast %d: %v", index, err)
		}
	}
	subscription, err := hub.Subscribe([]string{"ui"}, 1)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer subscription.Unsubscribe()
	if !subscription.ReplayGap || subscription.ReplayCount != 0 {
		t.Fatalf("subscription=%+v want explicit replay gap", subscription)
	}
}

func TestRealtimeHubSuppressesIdenticalState(t *testing.T) {
	hub := NewRealtimeHub()
	first, err := hub.Broadcast([]string{"metrics"}, realtimeMessage{EventName: "metrics-glance", StateKey: "metrics", HTML: "same"})
	if err != nil {
		t.Fatalf("first Broadcast: %v", err)
	}
	second, err := hub.Broadcast([]string{"metrics"}, realtimeMessage{EventName: "metrics-glance", StateKey: "metrics", HTML: "same"})
	if err != nil {
		t.Fatalf("second Broadcast: %v", err)
	}
	if first.MessageID == 0 || !second.Duplicate || second.MessageID != first.MessageID {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestRealtimeHubDoesNotSuppressIdenticalStateAcrossTopics(t *testing.T) {
	hub := NewRealtimeHub()
	message := realtimeMessage{EventName: "state-updated", StateKey: "shared-state", Reason: "same"}
	first, err := hub.Broadcast([]string{"metrics"}, message)
	if err != nil {
		t.Fatalf("first Broadcast: %v", err)
	}
	second, err := hub.Broadcast([]string{"jobs"}, message)
	if err != nil {
		t.Fatalf("second Broadcast: %v", err)
	}
	if second.Duplicate {
		t.Fatal("an identical state update for a different topic must be delivered")
	}
	if second.MessageID <= first.MessageID {
		t.Fatalf("second message id=%d want greater than first id=%d", second.MessageID, first.MessageID)
	}
}

func TestRealtimeHubBoundsStateFingerprintsToReplayWindow(t *testing.T) {
	hub := NewRealtimeHub(RealtimeHubOptions{ReplayCapacity: 8})
	for index := 0; index < 64; index++ {
		_, err := hub.Broadcast([]string{"jobs"}, realtimeMessage{
			EventName: "job-progress",
			StateKey:  fmt.Sprintf("job:%d", index),
			Reason:    "changed",
		})
		if err != nil {
			t.Fatalf("Broadcast %d: %v", index, err)
		}
	}
	hub.mu.Lock()
	fingerprintCount := len(hub.lastFingerprint)
	hub.mu.Unlock()
	if fingerprintCount > 8 {
		t.Fatalf("state fingerprints=%d want at most replay capacity 8", fingerprintCount)
	}
}

func TestParseRealtimeTopicsRejectsUnknownTopic(t *testing.T) {
	if _, err := parseRealtimeTopics("ui,secrets"); err == nil {
		t.Fatal("unknown realtime topic must fail")
	}
}
