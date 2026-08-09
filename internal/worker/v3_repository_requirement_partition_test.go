package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExistingRepositoryRequirementPartitionUsesOnlyStableTypedCalls(t *testing.T) {
	t.Parallel()
	var kinds []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			if model != "qwen-stable" {
				t.Fatalf("model=%q", model)
			}
			kinds = append(kinds, job.Kind)
			var input assemblyline.RequirementPartitionInput
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				return assemblyline.PortableResult{}, err
			}
			quotes := []string{}
			switch {
			case input.Mode == assemblyline.RequirementSplitFeature:
				quotes = []string{input.SourceText}
			case strings.Contains(input.SourceText, "audit logging"):
				quotes = []string{"audit logging"}
			case strings.Contains(input.SourceText, "CSV exports"):
				quotes = []string{"CSV exports"}
			}
			raw, err := json.Marshal(assemblyline.RequirementPartitionDecision{
				Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: quotes,
			})
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	decision, err := partitionCodingRequirements(
		runtime, "qwen-stable", "Add audit logging and CSV exports to the service.", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decision.FeatureQuotes, []string{"audit logging", "CSV exports"}) {
		t.Fatalf("requirements=%+v", decision)
	}
	for _, kind := range kinds {
		if kind != assemblyline.WorkRequirementPartition {
			t.Fatalf("non-direct partition work kind=%q", kind)
		}
	}
}
