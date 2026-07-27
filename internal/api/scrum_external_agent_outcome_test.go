package api

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func codexScrumJob(status, output string) model.JobDetails {
	return model.JobDetails{
		Job: model.Job{
			Status:   status,
			Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"codex"},"scrum_raw_play":true}`),
		},
		Steps: []model.Step{{Output: output}},
	}
}

func TestResolveScrumManagerOutcomeCodexCompletedMovesToReview(t *testing.T) {
	output := `{"agent":"codex","type":"started","message":"Codex external implementation session started"}
{"agent":"codex","type":"completed","message":"Codex external implementation session completed"}`
	job := codexScrumJob(model.JobStatusCompleted, output)
	outcome := resolveScrumManagerOutcome(job)
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want success for completed codex run", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "review" || transition.PlayState != "" {
		t.Fatalf("transition=%+v want review", transition)
	}
}

func TestResolveScrumManagerOutcomeCodexSubstantiveMessageMovesToReview(t *testing.T) {
	output := `{"agent":"codex","type":"started","message":"Codex external implementation session started"}
{"agent":"codex","type":"message","message":"Implemented the state machine fix"}
{"agent":"codex","type":"completed","message":"Codex external implementation session completed"}`
	job := codexScrumJob(model.JobStatusCompleted, output)
	outcome := resolveScrumManagerOutcome(job)
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want success", outcome)
	}
}

func TestResolveScrumManagerOutcomeTypedAgentErrorMovesToError(t *testing.T) {
	output := `{"agent":"codex","type":"started","message":"Codex external implementation session started"}
{"agent":"codex","type":"error","message":"spawn codex ENOENT"}`
	job := codexScrumJob(model.JobStatusCompleted, output)
	outcome := resolveScrumManagerOutcome(job)
	if outcome != ScrumOutcomeFailed {
		t.Fatalf("outcome=%q want failed for typed agent error", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "error" || transition.PlayState != "" {
		t.Fatalf("transition=%+v want error", transition)
	}
}

func TestResolveScrumManagerOutcomeCursorErrorStatusMovesToError(t *testing.T) {
	output := `{"type":"status","agent_id":"agent-8bf257b3","run_id":"run-67ad2a31","status":"ERROR"}`
	job := codexScrumJob(model.JobStatusCompleted, output)
	if outcome := resolveScrumManagerOutcome(job); outcome != ScrumOutcomeFailed {
		t.Fatalf("outcome=%q want failed for Cursor ERROR status", outcome)
	}
}

func TestResolveScrumPlayOutcomeCodexCompletedWithoutLLM(t *testing.T) {
	s := &Server{}
	job := codexScrumJob(model.JobStatusCompleted, "Codex external implementation session completed")
	outcome, note := s.resolveScrumPlayOutcome(t.Context(), job)
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q note=%q want success", outcome, note)
	}
}

func TestScrumSyncTerminalPlayOutputBeforeReview(t *testing.T) {
	line := `{"agent":"codex","type":"message","message":"patched scrum_manager.go"}`
	card := ScrumCard{Column: "in_progress", PlayState: scrumPlayRunning}
	job := codexScrumJob(model.JobStatusCompleted, line)
	updated := scrumSyncTerminalPlayOutput(card, job)
	if syncedAgentStreamLenFromChat(updated.Chat) != len(line) {
		t.Fatalf("expected agent output synced to channel before transition; chat=%+v", updated.Chat)
	}
}
