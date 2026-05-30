package api

import (
	"encoding/json"
	"strings"
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

func TestSyncRunningJobConsoleLogIncremental(t *testing.T) {
	card := ScrumCard{ConsoleLog: "job 1 queued\n"}
	job := model.JobDetails{
		Steps: []model.Step{{Output: "line one\n"}},
	}
	updated, ok := syncRunningJobConsoleLog(card, job)
	if !ok {
		t.Fatal("expected first sync")
	}
	if !strings.Contains(updated.ConsoleLog, "agent stream:") {
		t.Fatalf("console=%q", updated.ConsoleLog)
	}
	if !strings.Contains(updated.ConsoleLog, "line one") {
		t.Fatalf("console=%q", updated.ConsoleLog)
	}

	job.Steps = []model.Step{{Output: "line one\nline two\n"}}
	updated2, ok := syncRunningJobConsoleLog(updated, job)
	if !ok {
		t.Fatal("expected second sync")
	}
	if !strings.Contains(updated2.ConsoleLog, "line two") {
		t.Fatalf("console=%q", updated2.ConsoleLog)
	}
	if strings.Count(updated2.ConsoleLog, "agent stream:") != 1 {
		t.Fatalf("should not duplicate stream header: %q", updated2.ConsoleLog)
	}

	display := StripAgentStreamMarker(updated2.ConsoleLog)
	if strings.Contains(display, "[[agent-stream-len:") {
		t.Fatalf("marker leaked to display: %q", display)
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
	if outcome != ScrumOutcomePaused {
		t.Fatalf("outcome=%q want paused", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "assigned" || transition.PlayState != scrumPlayPaused {
		t.Fatalf("transition=%+v want assigned/paused", transition)
	}
}
