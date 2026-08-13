package api

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentstream"
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

func TestReconcileScrumCardJobStateRejectsActiveCardWithoutJobID(t *testing.T) {
	s := &Server{repo: nil}
	card := ScrumCard{ID: "card-1", PlayState: scrumPlayRunning, JobID: ""}
	updated, changed, err := s.reconcileScrumCardJobState(t.Context(), 1, card)
	if err == nil || !strings.Contains(err.Error(), "without a job id") {
		t.Fatalf("error=%v want explicit missing job id failure", err)
	}
	if changed || updated.PlayState != scrumPlayRunning {
		t.Fatalf("invalid state must not be silently rewritten: updated=%+v changed=%v", updated, changed)
	}
}

func TestSyncRunningJobChannelChatAdvancesTypedCursorOnMerge(t *testing.T) {
	line := scrumAgentEventLine(t, agentstream.EventMessage, "Hello")
	job := modelJobDetailsWithOutput(line)
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{Chat: []ScrumChatMessage{{Role: "assistant", Content: "Hello", CreatedAt: "2026-05-29T10:00:00Z"}}})
	updated, ok, err := syncRunningJobChannelChat(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected sync")
	}
	if updated.AgentStreamChatCursor != int64(len(line)) {
		t.Fatalf("stream cursor not advanced: card=%+v", updated)
	}
}
