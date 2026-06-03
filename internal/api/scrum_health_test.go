package api

import (
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestScrumCardHealthState(t *testing.T) {
	card := ScrumCard{PlayState: scrumPlayRunning, UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	if got := scrumCardHealthState(card, model.JobStatusRunning, nil); got != "active" {
		t.Fatalf("running health=%q want active", got)
	}
	if got := scrumCardHealthState(card, model.JobStatusCompleted, nil); got != "done" {
		t.Fatalf("completed health=%q want done", got)
	}
	if got := scrumCardHealthState(card, model.JobStatusFailed, nil); got != "errored" {
		t.Fatalf("failed health=%q want errored", got)
	}
}

func TestScrumCardHealthStateStalled(t *testing.T) {
	card := ScrumCard{
		PlayState: scrumPlayRunning,
		UpdatedAt: time.Now().UTC().Add(-scrumHealthStalledAge - time.Minute).Format(time.RFC3339),
	}
	if got := scrumCardHealthState(card, model.JobStatusRunning, nil); got != "stalled" {
		t.Fatalf("stalled health=%q want stalled", got)
	}
}

func TestScrumHealthTTL(t *testing.T) {
	if got := scrumHealthTTL([]scrumCardHealth{{Health: "idle"}}); got != scrumHealthIdleTTLMS {
		t.Fatalf("idle ttl=%d want %d", got, scrumHealthIdleTTLMS)
	}
	if got := scrumHealthTTL([]scrumCardHealth{{Health: "idle"}, {Health: "active"}}); got != scrumHealthActiveTTLMS {
		t.Fatalf("active ttl=%d want %d", got, scrumHealthActiveTTLMS)
	}
}
