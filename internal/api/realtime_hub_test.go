package api

import (
	"encoding/json"
	"testing"
)

func TestRealtimeDuplicatesCompareActualContentWithinTopicScope(t *testing.T) {
	for _, fixture := range []realtimeMessage{
		{EventName: "job_changed", StateKey: "job:1", JobID: 1, Summary: "Queued"},
		{EventName: "queue_changed", StateKey: "queue:2", PlayQueue: map[string]any{"position": 2, "state": "waiting"}},
	} {
		t.Run(fixture.EventName, func(t *testing.T) {
			hub := NewRealtimeHub()
			live, err := hub.Subscribe([]string{realtimeTopicJobs}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer live.Unsubscribe()
			first, err := hub.Broadcast([]string{realtimeTopicJobs, realtimeTopicUI}, fixture)
			if err != nil || first.Duplicate || first.MessageID != 1 || first.DeliveredClients != 1 {
				t.Fatalf("first publication = %#v / %v", first, err)
			}
			received := <-live.Messages
			fixture.ID, fixture.OccurredAt = 999, "different transport time"
			repeat, err := hub.Broadcast([]string{realtimeTopicUI, realtimeTopicJobs}, fixture)
			if err != nil || !repeat.Duplicate || repeat.MessageID != first.MessageID || repeat.DeliveredClients != 0 {
				t.Fatalf("transport metadata or topic order defeated deduplication: %#v / %v", repeat, err)
			}
			select {
			case <-live.Messages:
				t.Fatal("duplicate was delivered")
			default:
			}
			fixture.Summary = "Changed"
			if fixture.PlayQueue != nil {
				fixture.PlayQueue["state"] = "running"
			}
			changed, err := hub.Broadcast([]string{realtimeTopicJobs, realtimeTopicUI}, fixture)
			if err != nil || changed.Duplicate || changed.MessageID != 2 {
				t.Fatalf("changed content was suppressed: %#v / %v", changed, err)
			}
			cursor := uint64(0)
			replay, err := hub.Subscribe([]string{realtimeTopicJobs}, &cursor)
			if err != nil || replay.ReplayGap || replay.ReplayCount != 2 {
				t.Fatalf("replay = %#v / %v", replay, err)
			}
			defer replay.Unsubscribe()
			if firstReplay := <-replay.Messages; string(firstReplay) != string(received) {
				t.Fatal("later content mutation changed the earlier published message")
			}
			var latest realtimeMessage
			if err := json.Unmarshal(<-replay.Messages, &latest); err != nil || latest.ID != 2 || latest.Summary != "Changed" {
				t.Fatalf("replay lost changed state: %#v / %v", latest, err)
			}
			otherScope, err := hub.Broadcast([]string{realtimeTopicUI}, fixture)
			if err != nil || otherScope.Duplicate || otherScope.MessageID != 3 {
				t.Fatalf("different topic scope was suppressed: %#v / %v", otherScope, err)
			}
		})
	}
}

func TestRealtimeRetainedValuesAreBoundedByReplayHistory(t *testing.T) {
	hub := NewRealtimeHub()
	hub.replayCapacity = 2
	for _, item := range []struct {
		key, value string
		id         uint64
		duplicate  bool
	}{
		{"a", "first", 1, false}, {"b", "first", 2, false}, {"a", "second", 3, false},
		{"a", "second", 3, true}, {"c", "first", 4, false}, {"a", "second", 3, true},
		{"d", "first", 5, false}, {"a", "second", 6, false},
	} {
		result, err := hub.Broadcast([]string{realtimeTopicJobs}, realtimeMessage{
			EventName: "changed", StateKey: item.key, Summary: item.value,
		})
		if err != nil || result.MessageID != item.id || result.Duplicate != item.duplicate {
			t.Fatalf("publication %#v = %#v / %v", item, result, err)
		}
		if len(hub.lastState) > 2 || len(hub.history) > 2 {
			t.Fatal("retained values outlived the bounded replay history")
		}
	}
	for repeat := 0; repeat < 2; repeat++ {
		result, err := hub.Broadcast([]string{realtimeTopicJobs}, realtimeMessage{EventName: "notification"})
		if err != nil || result.Duplicate {
			t.Fatalf("stateless notification was deduplicated: %#v / %v", result, err)
		}
	}
}

func TestInvalidRealtimeContentDoesNotAdvanceState(t *testing.T) {
	hub := NewRealtimeHub()
	_, err := hub.Broadcast([]string{realtimeTopicJobs}, realtimeMessage{
		EventName: "changed", StateKey: "invalid", PlayQueue: map[string]any{"unsupported": make(chan int)},
	})
	if err == nil || hub.nextMessageID != 0 || len(hub.lastState) != 0 || len(hub.history) != 0 {
		t.Fatalf("invalid content changed hub state: %v", err)
	}
}
