package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentstream"
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

func TestSyncRunningJobConsoleLogIncremental(t *testing.T) {
	lineOne := scrumAgentEventLine(t, agentstream.EventMessage, "line one")
	lineTwo := scrumAgentEventLine(t, agentstream.EventMessage, "line two")
	job := model.JobDetails{
		Job:   model.Job{ID: 4, Status: model.JobStatusRunning},
		Steps: []model.Step{{Action: "external_agent_execute", Output: lineOne + "\n"}},
	}
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{ConsoleLog: "job 1 queued\n"})
	updated, ok, err := syncRunningJobConsoleLog(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected first sync")
	}
	if !strings.Contains(updated.ConsoleLog, "agent stream:") {
		t.Fatalf("console=%q", updated.ConsoleLog)
	}
	if !strings.Contains(updated.ConsoleLog, "line one") {
		t.Fatalf("console=%q", updated.ConsoleLog)
	}

	job.Steps = []model.Step{{Action: "external_agent_execute", Output: lineOne + "\n" + lineTwo + "\n"}}
	updated2, ok, err := syncRunningJobConsoleLog(updated, job)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected second sync")
	}
	if !strings.Contains(updated2.ConsoleLog, "line two") {
		t.Fatalf("console=%q", updated2.ConsoleLog)
	}
	if strings.Count(updated2.ConsoleLog, "agent stream:") != 1 {
		t.Fatalf("should not duplicate stream header: %q", updated2.ConsoleLog)
	}

	if updated2.AgentStreamConsoleCursor != int64(len(lineOne+"\n"+lineTwo+"\n")) {
		t.Fatalf("console cursor=%d", updated2.AgentStreamConsoleCursor)
	}
}

func TestResolveScrumPlayOutcomeFailedJob(t *testing.T) {
	details := model.JobDetails{
		Job: model.Job{
			Status:   model.JobStatusFailed,
			Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"codex"},"scrum_raw_play":true}`),
		},
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
		Job: model.Job{
			Status:   model.JobStatusCanceled,
			Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"codex"},"scrum_raw_play":true}`),
		},
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
		Job: model.Job{
			Status:   model.JobStatusFailed,
			Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"codex"},"scrum_raw_play":true}`),
		},
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
