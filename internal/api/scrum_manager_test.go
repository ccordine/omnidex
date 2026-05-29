package api

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestParseScrumManagerOutcome(t *testing.T) {
	outcome, ok := parseScrumManagerOutcome("All good.\nSCRUM_STATUS: success\n")
	if !ok || outcome != ScrumOutcomeSuccess {
		t.Fatalf("parse success = %q ok=%v", outcome, ok)
	}
	outcome, ok = parseScrumManagerOutcome(`{"scrum_status":"blocked","reason":"waiting on API key"}`)
	if !ok || outcome != ScrumOutcomeBlocked {
		t.Fatalf("parse blocked json = %q ok=%v", outcome, ok)
	}
}

func TestResolveScrumManagerOutcomeFromJob(t *testing.T) {
	details := model.JobDetails{
		Job: model.Job{Status: model.JobStatusCompleted},
		Steps: []model.Step{{
			Output: "Implemented feature.\nSCRUM_STATUS: blocked\nNeed credentials.",
		}},
	}
	outcome := resolveScrumManagerOutcome(details)
	if outcome != ScrumOutcomeBlocked {
		t.Fatalf("resolve outcome = %q want blocked", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "blocked" {
		t.Fatalf("transition column = %q want blocked", transition.Column)
	}
}

func TestScrumColumnForOutcomeFailed(t *testing.T) {
	transition := scrumColumnForOutcome(ScrumOutcomeFailed)
	if transition.Column != "blocked" || transition.PlayState != "" {
		t.Fatalf("failed transition = %+v", transition)
	}
}

func TestResolveScrumPlayOutcomeFailedJob(t *testing.T) {
	details := model.JobDetails{
		Job: model.Job{
			Status:   model.JobStatusFailed,
			Metadata: json.RawMessage(`{"source":"omni-scrum","execution_agent":"codex","scrum_raw_play":true}`),
		},
	}
	outcome := resolveScrumManagerOutcome(details)
	if outcome != ScrumOutcomeBlocked {
		t.Fatalf("outcome=%q want blocked", outcome)
	}
}
