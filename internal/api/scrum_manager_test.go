package api

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestScrumColumnForOutcomeFailed(t *testing.T) {
	transition := scrumColumnForOutcome(ScrumOutcomeFailed)
	if transition.Column != "error" || transition.PlayState != "" {
		t.Fatalf("failed transition = %+v", transition)
	}
}

func TestScrumColumnForUnknownOutcomeFailsClosed(t *testing.T) {
	transition := scrumColumnForOutcome(ScrumManagerOutcome("unknown"))
	if transition.Column != "error" || !strings.Contains(transition.ConsoleNote, "invalid outcome") {
		t.Fatalf("transition=%+v want fail-closed error column", transition)
	}
}

func TestResolveScrumPlayOutcomeFailedJob(t *testing.T) {
	details := model.JobDetails{
		Job: model.Job{Status: model.JobStatusFailed},
	}
	outcome, err := resolveScrumManagerOutcome(details)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeFailed {
		t.Fatalf("outcome=%q want failed", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "error" || transition.PlayState != "" {
		t.Fatalf("transition=%+v want error", transition)
	}
}

func TestResolveScrumPlayOutcomeCanceledJobMovesToError(t *testing.T) {
	details := model.JobDetails{
		Job:   model.Job{Status: model.JobStatusCanceled},
		Steps: []model.Step{{Output: "connection lost"}},
	}
	outcome, err := resolveScrumManagerOutcome(details)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeFailed {
		t.Fatalf("outcome=%q want failed", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "error" || transition.PlayState != "" {
		t.Fatalf("transition=%+v want error", transition)
	}
}

func TestResolveScrumPlayOutcomeFailureStatusOverridesSuccessText(t *testing.T) {
	details := model.JobDetails{
		Job:   model.Job{Status: model.JobStatusFailed},
		Steps: []model.Step{{Output: "SCRUM_STATUS: success"}},
	}
	outcome, err := resolveScrumManagerOutcome(details)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeFailed {
		t.Fatalf("outcome=%q want failed", outcome)
	}
}
