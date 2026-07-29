package api

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestFeedbackAuthorityRejectsMismatchedJob(t *testing.T) {
	err := validateSameJobAuthority(41, model.Job{ID: 42})
	if err == nil || !strings.Contains(err.Error(), "expected job 41") {
		t.Fatalf("mismatched job error=%v", err)
	}
	if err := validateSameJobAuthority(41, model.Job{ID: 41}); err != nil {
		t.Fatalf("same job rejected: %v", err)
	}
}

func TestFeedbackHTTPHasNoSuccessorJobFallback(t *testing.T) {
	for _, path := range []string{"server_llm.go", "server_jobs.go", "scrum_channel_agent.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"superseded_job_id",
			"Job superseded by revised user authority",
			"replaced job",
			"controlledJob.ID != jobID",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s contains forbidden successor-job fallback %q", path, forbidden)
			}
		}
	}
}
