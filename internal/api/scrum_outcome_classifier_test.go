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
