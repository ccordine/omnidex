package api

import (
	"testing"
)

func TestDbScrumCardToAPIPreservesSyncMarkers(t *testing.T) {
	raw := ScrumCard{
		Chat: []ScrumChatMessage{
			{Role: "assistant", Content: "working\n[[agent-stream-len:42]]"},
			{Role: "system", Content: "[[context-sync:7]]"},
		},
	}
	// dbScrumCardToAPI path unmarshals then returns — markers must survive for server-side sync.
	out := ScrumCard{Chat: append([]ScrumChatMessage(nil), raw.Chat...)}
	foundStream := false
	foundContext := false
	for _, msg := range out.Chat {
		if msg.Content == "[[context-sync:7]]" {
			foundContext = true
		}
		if msg.Content == "working\n[[agent-stream-len:42]]" {
			foundStream = true
		}
	}
	if !foundStream || !foundContext {
		t.Fatalf("markers stripped: %+v", out.Chat)
	}
}

func TestReconcileScrumCardJobStateClearsStaleRunning(t *testing.T) {
	s := &Server{repo: nil}
	card := ScrumCard{PlayState: scrumPlayRunning, JobID: ""}
	updated, ok := s.reconcileScrumCardJobState(t.Context(), 0, card)
	if !ok || updated.PlayState != "" {
		t.Fatalf("updated=%+v ok=%v", updated, ok)
	}
}

func TestSyncRunningJobChannelChatAdvancesMarkerOnMerge(t *testing.T) {
	line := `{"agent":"cursor","type":"message","message":"{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Hello\"}]}}"}`
	card := ScrumCard{Chat: []ScrumChatMessage{{Role: "assistant", Content: "Hello", CreatedAt: "2026-05-29T10:00:00Z"}}}
	job := modelJobDetailsWithOutput(line)
	updated, ok := syncRunningJobChannelChat(card, job)
	if !ok {
		t.Fatal("expected sync")
	}
	if syncedAgentStreamLenFromChat(updated.Chat) != len(line) {
		t.Fatalf("stream marker not advanced: chat=%+v", updated.Chat)
	}
}
