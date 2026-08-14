package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationRequirementsUseOneIntactProductionSemanticCall(t *testing.T) {
	t.Parallel()
	const request = "Build a browser catalog with grouped records and printable summaries."
	counts := make(map[string]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			counts[string(job.Kind)]++
			var candidate any
			switch job.Kind {
			case assemblyline.WorkApplicationClassify:
				if modelName != "surface-model" {
					return assemblyline.PortableResult{}, fmt.Errorf("surface model=%q", modelName)
				}
				candidate = assemblyline.ApplicationClassification{
					Schema:  assemblyline.ApplicationClassificationSchemaV1,
					Surface: assemblyline.ApplicationSurfaceBrowser,
				}
			case assemblyline.WorkApplicationRequirements:
				if modelName != "requirement-model" {
					return assemblyline.PortableResult{}, fmt.Errorf("requirement model=%q", modelName)
				}
				var input assemblyline.ApplicationRequirementInterpretationInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.UserRequest != request {
					return assemblyline.PortableResult{}, fmt.Errorf(
						"requirement authority=%q want intact request", input.UserRequest,
					)
				}
				candidate = assemblyline.ApplicationRequirementInterpretation{
					Schema: assemblyline.ApplicationRequirementInterpretationSchemaV1,
					Items: []assemblyline.ApplicationRequirementItem{
						{Kind: assemblyline.ApplicationRequirementFeature, SourceQuote: "printable summaries"},
						{Kind: assemblyline.ApplicationRequirementProduct, SourceQuote: "browser catalog"},
						{Kind: assemblyline.ApplicationRequirementFeature, SourceQuote: "grouped records"},
					},
				}
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected semantic work kind %q", job.Kind)
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	specification, err := runDirectCodingApplicationInterpreter(
		runtime, "requirement-model", "surface-model", "artifact-model", request, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if counts[string(assemblyline.WorkApplicationRequirements)] != 1 ||
		counts["application_identity"] != 0 || counts["requirement_partition"] != 0 {
		t.Fatalf("semantic call counts=%v", counts)
	}
	if specification.ProductQuote != "browser catalog" ||
		!reflect.DeepEqual(specification.Requirements, []assemblyline.Requirement{
			{ID: "requirement_001", SourceQuote: "grouped records"},
			{ID: "requirement_002", SourceQuote: "printable summaries"},
		}) {
		t.Fatalf("specification=%+v", specification)
	}
}

func TestInvalidApplicationRequirementInterpretationFailsAfterOneCall(t *testing.T) {
	t.Parallel()
	const request = "Build a browser catalog with grouped records."
	requirementExecutions := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			var candidate any
			switch job.Kind {
			case assemblyline.WorkApplicationClassify:
				candidate = assemblyline.ApplicationClassification{
					Schema:  assemblyline.ApplicationClassificationSchemaV1,
					Surface: assemblyline.ApplicationSurfaceBrowser,
				}
			case assemblyline.WorkApplicationRequirements:
				requirementExecutions++
				candidate = assemblyline.ApplicationRequirementInterpretation{
					Schema: assemblyline.ApplicationRequirementInterpretationSchemaV1,
					Items: []assemblyline.ApplicationRequirementItem{
						{Kind: assemblyline.ApplicationRequirementProduct, SourceQuote: "browser catalog"},
						{Kind: assemblyline.ApplicationRequirementFeature, SourceQuote: "invented grouping"},
					},
				}
			case assemblyline.WorkResponseCorrection:
				requirementExecutions++
				candidate = map[string]any{"items": []any{}}
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected semantic work kind %q", job.Kind)
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	_, err := runDirectCodingApplicationInterpreter(
		runtime, "requirement-model", "surface-model", "artifact-model", request, nil,
	)
	if err == nil {
		t.Fatal("invalid aggregate interpretation succeeded")
	}
	if requirementExecutions != 1 {
		t.Fatalf("invalid aggregate made %d requirement-model calls", requirementExecutions)
	}
}
