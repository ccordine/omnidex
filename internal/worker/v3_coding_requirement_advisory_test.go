package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestRequirementPartitionUsesBriefingAdvisoryAndAuthoritativeSynthesis(t *testing.T) {
	t.Parallel()

	input := assemblyline.RequirementPartitionInput{
		SourceText: "Build a catalog with grouped records and a saved filter.",
		Mode:       assemblyline.RequirementExtractFeatures,
	}
	var executed []assemblyline.WorkKind
	var advisoryJobs []assemblyline.PortableJob
	var synthesisPrompt string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, AdvisoryModel: "deepseek-r1:8b",
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			executed = append(executed, job.Kind)
			var candidate any
			switch job.Kind {
			case assemblyline.WorkRequirementBriefing:
				candidate = assemblyline.RequirementPartitionBriefingDecision{
					Schema: assemblyline.RequirementPartitionBriefingSchemaV1,
					Lens:   assemblyline.RequirementLensCoverage,
				}
			case assemblyline.WorkRequirementSynthesis:
				prompt, _, err := assemblyline.RenderPortableJob(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				synthesisPrompt = prompt
				candidate = partitionDecision("grouped records", "a saved filter")
			default:
				t.Fatalf("unexpected structured job kind %q", job.Kind)
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
		Advise: func(job assemblyline.PortableJob, model string) (llm.AdvisoryResponse, error) {
			advisoryJobs = append(advisoryJobs, job)
			if model != "deepseek-r1:8b" {
				t.Fatalf("advisory model=%q", model)
			}
			return llm.AdvisoryResponse{
				Thinking: "private native reasoning that must remain evidence-only",
				Content:  "Check both explicit feature spans and preserve source order.",
			}, nil
		},
	}

	decision, err := partitionRequirementFeatures(runtime, "stable-model", input, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.FeatureQuotes) != 2 {
		t.Fatalf("decision=%#v", decision)
	}
	if len(executed) != 2 || executed[0] != assemblyline.WorkRequirementBriefing || executed[1] != assemblyline.WorkRequirementSynthesis {
		t.Fatalf("structured jobs=%#v", executed)
	}
	if len(advisoryJobs) != 1 || advisoryJobs[0].Kind != assemblyline.WorkRequirementAdvisory {
		t.Fatalf("advisory jobs=%#v", advisoryJobs)
	}
	if !strings.Contains(synthesisPrompt, "Check both explicit feature spans") {
		t.Fatalf("synthesis omitted final advisory memo:\n%s", synthesisPrompt)
	}
	if strings.Contains(synthesisPrompt, "private native reasoning") {
		t.Fatalf("synthesis leaked native thinking:\n%s", synthesisPrompt)
	}
}

func TestRequirementPartitionFailsWhenAdvisoryRuntimeIsUnavailable(t *testing.T) {
	t.Parallel()

	input := assemblyline.RequirementPartitionInput{
		SourceText: "grouped records", Mode: assemblyline.RequirementSplitFeature,
	}
	runtime := typedWorkerRuntime{Context: context.Background(), MaxAttempts: 3}
	_, err := partitionRequirementFeatures(runtime, "stable-model", input, 1, nil)
	if err == nil || !strings.Contains(err.Error(), "advisory") {
		t.Fatalf("error=%v", err)
	}
}
