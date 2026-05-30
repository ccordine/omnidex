package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestParseScrumOutcomeClassification(t *testing.T) {
	raw := `Here is my analysis {"outcome":"success","confidence":0.91,"reason":"task completed with working changes","real_error":false}`
	got, ok := parseScrumOutcomeClassification(raw)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if got.Outcome != ScrumOutcomeSuccess || got.Confidence != 0.91 || got.RealError {
		t.Fatalf("got=%+v", got)
	}
}

func TestParseScrumOutcomeClassificationRejectsInvalidOutcome(t *testing.T) {
	_, ok := parseScrumOutcomeClassification(`{"outcome":"maybe","confidence":0.9,"reason":"x"}`)
	if ok {
		t.Fatal("expected invalid outcome rejected")
	}
}

func TestScrumBaselinePlayOutcomeCompletedInProgress(t *testing.T) {
	job := modelJobDetails(model.JobStatusCompleted, "Still working on edge cases.\nSCRUM_STATUS: in_progress\n")
	outcome := scrumBaselinePlayOutcome(job)
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q want success", outcome)
	}
}

func TestResolveScrumPlayOutcomeCompletedRawPlayDefaultsReviewWithoutLLM(t *testing.T) {
	s := &Server{}
	job := modelJobDetails(model.JobStatusCompleted, "Codex external implementation session completed")
	outcome, note := s.resolveScrumPlayOutcome(t.Context(), job)
	if outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q note=%q want success", outcome, note)
	}
	transition := scrumColumnForOutcome(outcome)
	if transition.Column != "review" || transition.PlayState != "" {
		t.Fatalf("transition=%+v want review with no play state", transition)
	}
}

func TestStabilizeCompletedScrumOutcomeIgnoresInProgressScan(t *testing.T) {
	job := modelJobDetails(model.JobStatusCompleted, "Codex external implementation session completed")
	outcome, ok := stabilizeCompletedScrumOutcome(job, ScrumOutcomeSuccess, scrumOutcomeClassification{
		Outcome:    ScrumOutcomeInProgress,
		Confidence: 0.9,
		Reason:     "agent mentioned continuing work",
	})
	if !ok || outcome != ScrumOutcomeSuccess {
		t.Fatalf("outcome=%q ok=%v want success stabilization", outcome, ok)
	}
}

func TestBuildScrumOutcomeClassifierUserPromptTruncates(t *testing.T) {
	long := make([]byte, 5000)
	for i := range long {
		long[i] = 'x'
	}
	job := modelJobDetails(model.JobStatusCompleted, string(long))
	job.Job.Metadata = json.RawMessage(`{"source":"omni-scrum","scrum_card_title":"Fix auth"}`)
	prompt := buildScrumOutcomeClassifierUserPrompt(job, ScrumOutcomeSuccess)
	if len(prompt) > scrumOutcomeClassifierMaxChars+800 {
		t.Fatalf("prompt too long: %d", len(prompt))
	}
	if !strings.Contains(prompt, "Fix auth") {
		t.Fatalf("missing card title: %q", prompt[:200])
	}
}

func modelJobDetails(status, output string) model.JobDetails {
	return model.JobDetails{
		Job: model.Job{
			Status:   status,
			Metadata: json.RawMessage(`{"source":"omni-scrum","scrum_raw_play":true}`),
		},
		Steps: []model.Step{{Output: output}},
	}
}
