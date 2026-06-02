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
			Metadata: json.RawMessage(`{"source":"omni-scrum","execution_agent":"codex","scrum_raw_play":true}`),
		},
		Steps: []model.Step{{Output: output}},
	}
}

func TestResolveScrumManagerOutcomeCodexBoilerplateOnlyStaysPaused(t *testing.T) {
	output := `{"agent":"codex","type":"started","message":"Codex external implementation session started"}
{"agent":"codex","type":"completed","message":"Codex external implementation session completed"}`
	job := codexScrumJob(model.JobStatusCompleted, output)
	outcome := resolveScrumManagerOutcome(job)
	if outcome != ScrumOutcomePaused {
		t.Fatalf("outcome=%q want paused for boilerplate-only codex run", outcome)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "assigned" || transition.PlayState != scrumPlayPaused {
		t.Fatalf("transition=%+v want assigned/paused", transition)
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

func TestResolveScrumManagerOutcomeCodexSpawnFailureStaysPaused(t *testing.T) {
	output := "Codex external implementation session started\nError: spawn codex ENOENT\nexternal agent session ended"
	job := codexScrumJob(model.JobStatusCompleted, output)
	outcome := resolveScrumManagerOutcome(job)
	if outcome != ScrumOutcomePaused {
		t.Fatalf("outcome=%q want paused for spawn failure output", outcome)
	}
}

func TestResolveScrumPlayOutcomeCodexBoilerplateWithoutLLM(t *testing.T) {
	s := &Server{}
	job := codexScrumJob(model.JobStatusCompleted, "Codex external implementation session completed")
	outcome, note := s.resolveScrumPlayOutcome(t.Context(), job)
	if outcome != ScrumOutcomePaused {
		t.Fatalf("outcome=%q note=%q want paused", outcome, note)
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
