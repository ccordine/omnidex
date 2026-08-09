package queue

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestReplanSourceHasOneGenerationPathAndNoResetFallback(t *testing.T) {
	paths := []string{
		"repository_replan.go", "repository_replan_commit.go",
		"job_generation_replan.go", "job_generation_store.go",
	}
	var source strings.Builder
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source.Write(raw)
		source.WriteByte('\n')
	}
	text := source.String()
	for _, forbidden := range []string{
		"INSERT INTO step_contexts",
		`"replan_feedback"`,
		`"user_feedback"`,
		"output = NULL",
		"started_at = NULL",
		"action IN ('v3_coding', 'v3_subtask', 'v3_planning', 'plan')",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("replan source contains forbidden reset/fallback path %q", forbidden)
		}
	}
	updateToParameter := regexp.MustCompile(`(?is)UPDATE\s+job_steps\s+SET\s+status\s*=\s*\$[0-9]+`)
	if updateToParameter.MatchString(text) {
		t.Fatal("replan must not reset existing steps to a parameterized status")
	}
	for _, required := range []string{
		"INSERT INTO job_generations",
		"superseded_at_generation",
		"current_generation",
		"feedback_sha256",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("replan source is missing generation authority %q", required)
		}
	}
}
