package api

import (
	"errors"
	"testing"
)

func TestRealtimeHubRejectsClientsOverLimit(t *testing.T) {
	hub := NewRealtimeHub(RealtimeHubOptions{MaxClients: 1})

	_, _, unsubscribe, err := hub.Subscribe([]string{"ui"})
	if err != nil {
		t.Fatalf("first Subscribe() error: %v", err)
	}
	defer unsubscribe()

	_, _, _, err = hub.Subscribe([]string{"ui"})
	if !errors.Is(err, ErrRealtimeHubFull) {
		t.Fatalf("second Subscribe() error=%v want ErrRealtimeHubFull", err)
	}
}

func TestRealtimeHubUnsubscribeFreesCapacity(t *testing.T) {
	hub := NewRealtimeHub(RealtimeHubOptions{MaxClients: 1})

	_, _, unsubscribe, err := hub.Subscribe([]string{"ui"})
	if err != nil {
		t.Fatalf("first Subscribe() error: %v", err)
	}
	unsubscribe()

	_, _, secondUnsubscribe, err := hub.Subscribe([]string{"ui"})
	if err != nil {
		t.Fatalf("second Subscribe() error: %v", err)
	}
	secondUnsubscribe()
}
