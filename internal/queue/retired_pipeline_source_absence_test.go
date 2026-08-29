package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRetiredPipelineExecutionAuthorityIsAbsentFromProductionSource(t *testing.T) {
	files := []string{
		"../model/model.go",
		"repository.go",
		"repository_pipeline.go",
		"repository_replan.go",
		"repository_step_claim.go",
		"../worker/objective_turn_types.go",
		"../worker/step_runner.go",
		"../api/job_collection.go",
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read production authority %s: %v", file, err)
		}
		source := string(raw)
		for _, forbidden := range []string{"PipelineAssistant", "PipelineStory"} {
			if strings.Contains(source, forbidden) {
				t.Errorf("production authority %s still contains retired execution symbol %q", file, forbidden)
			}
		}
	}

	raw, err := os.ReadFile("repository_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{`"assistant"`, `"story"`, `"agent"`} {
		if strings.Contains(string(raw), retired) {
			t.Errorf("pipeline registry contains retired executable literal %s", retired)
		}
	}
}
