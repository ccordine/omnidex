package api

import (
	"strings"
	"testing"
)

func TestReconcileScrumCardJobStateRejectsActiveCardWithoutJobID(t *testing.T) {
	s := &Server{repo: nil}
	card := ScrumCard{ID: "card-1", PlayState: scrumPlayRunning, JobID: ""}
	updated, changed, outcome, err := s.reconcileScrumCardJobState(t.Context(), 1, card)
	if err == nil || !strings.Contains(err.Error(), "without a job id") {
		t.Fatalf("error=%v want explicit missing job id failure", err)
	}
	if changed || outcome != "" || updated.PlayState != scrumPlayRunning {
		t.Fatalf("invalid state must not be silently rewritten: updated=%+v changed=%v outcome=%q", updated, changed, outcome)
	}
}
