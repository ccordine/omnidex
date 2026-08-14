package queue

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestTypedScrumMetadataUsesOnlyDirectCoding(t *testing.T) {
	meta := []byte(`{"source":"omni-scrum","project_id":4,"scrum_card_id":"card-4","scrum_card_title":"Card","scrum_card_description":"","scrum_checklist":"","scrum_test_criteria":"","scrum_return_column":"","scrum_channel_origin":false,"scrum_channel_operation_id":"","model_config":{},"telemetry_run_id":"00000000-0000-4000-8000-000000000001"}`)
	got, err := stepsForJob(model.PipelineScrum, "implement card", meta)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	want := []stepSeed{{action: "v3_coding", sortIndex: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Scrum metadata selected another runtime: got=%+v want=%+v", got, want)
	}
}

func TestQueueSourceHasNoScrumExternalAgentBranch(t *testing.T) {
	raw, err := os.ReadFile("repository_pipeline.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "if pipeline == model.PipelineScrum {")
	if start < 0 {
		t.Fatal("Scrum step derivation branch is missing")
	}
	end := strings.Index(source[start:], "\n\tif pipeline == model.PipelineCoding")
	if end < 0 {
		t.Fatal("Scrum step derivation boundary is missing")
	}
	body := source[start : start+end]
	for _, forbidden := range []string{"agentCfg", "external_agent_execute", "IsExternal"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("Scrum step derivation retains model/runtime selection %q", forbidden)
		}
	}
}

func TestScrumChannelOriginDoesNotChangeDirectCodingTransport(t *testing.T) {
	meta := []byte(`{"source":"omni-scrum","project_id":4,"scrum_card_id":"card-4","scrum_card_title":"Card","scrum_card_description":"","scrum_checklist":"","scrum_test_criteria":"","scrum_return_column":"","scrum_channel_origin":true,"scrum_channel_operation_id":"lifecycle_operation_1111111111111111111111111111111111111111111111111111111111111111","model_config":{},"telemetry_run_id":"00000000-0000-4000-8000-000000000001"}`)
	steps, err := stepsForJob(model.PipelineScrum, "continue card", meta)
	if err != nil {
		t.Fatal(err)
	}
	if got := stepActions(steps); !reflect.DeepEqual(got, []string{"v3_coding"}) {
		t.Fatalf("Scrum channel origin changed typed coding transport: %#v", got)
	}
}
