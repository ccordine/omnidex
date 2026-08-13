package api

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/agentstream"
	"github.com/gryph/omnidex/internal/model"
)

func codexScrumJob(status, output string) model.JobDetails {
	return model.JobDetails{
		Job: model.Job{
			ID:       77,
			Status:   status,
			Metadata: json.RawMessage(`{"source":"omni-scrum","agent_config":{"agent_system":"codex"},"scrum_raw_play":true}`),
		},
		Steps: []model.Step{{Action: "external_agent_execute", Output: output}},
	}
}

func TestResolveScrumManagerOutcomeCodexCompletedMovesToReview(t *testing.T) {
	output := `{"agent":"codex","type":"started","message":"Codex external implementation session started"}
{"agent":"codex","type":"completed","message":"Codex external implementation session completed"}`
	job := codexScrumJob(model.JobStatusCompleted, output)
	outcome, err := resolveScrumManagerOutcome(job)
	if err != nil {
		t.Fatal(err)
	}
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
	outcome, err := resolveScrumManagerOutcome(job)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want success", outcome)
	}
}

func TestResolveScrumManagerOutcomeIgnoresAgentErrorContent(t *testing.T) {
	output := `{"agent":"codex","type":"started","message":"Codex external implementation session started"}
{"agent":"codex","type":"error","message":"spawn codex ENOENT"}`
	job := codexScrumJob(model.JobStatusCompleted, output)
	outcome, err := resolveScrumManagerOutcome(job)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want typed completed lifecycle to win", outcome)
	}
}

func TestResolveScrumManagerOutcomeIgnoresCursorStatusContent(t *testing.T) {
	output := `{"type":"status","agent_id":"agent-8bf257b3","run_id":"run-67ad2a31","status":"ERROR"}`
	job := codexScrumJob(model.JobStatusCompleted, output)
	outcome, err := resolveScrumManagerOutcome(job)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want typed completed lifecycle to win", outcome)
	}
}

func TestResolveScrumPlayOutcomeCodexCompletedWithoutLLM(t *testing.T) {
	s := &Server{}
	job := codexScrumJob(model.JobStatusCompleted, "Codex external implementation session completed")
	outcome, err := s.resolveScrumPlayOutcome(t.Context(), job)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want success", outcome)
	}
}

func TestScrumSyncTerminalPlayOutputBeforeReview(t *testing.T) {
	line := scrumAgentEventLine(t, agentstream.EventMessage, "patched scrum_manager.go")
	job := codexScrumJob(model.JobStatusCompleted, line)
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{Column: "in_progress", PlayState: scrumPlayRunning})
	updated, err := scrumSyncTerminalPlayOutput(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AgentStreamChatCursor != int64(len(line)) {
		t.Fatalf("expected typed cursor advanced before transition; card=%+v", updated)
	}
}
