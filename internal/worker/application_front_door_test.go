package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationFrontDoorSkipsCeremonialReviewForEmptyWorkspace(t *testing.T) {
	t.Parallel()
	const request = "Build a browser counter that shows the count and can increment, decrement, and reset it."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[assemblyline.WorkKind]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			counts[job.Kind]++
			var candidate any
			switch job.Kind {
			case assemblyline.WorkApplicationClassify:
				candidate = assemblyline.ApplicationClassification{
					Schema:  assemblyline.ApplicationClassificationSchemaV1,
					Surface: assemblyline.ApplicationSurfaceBrowser,
				}
			case assemblyline.WorkApplicationIntent:
				candidate = assemblyline.ApplicationIntentCandidate{
					Schema:         assemblyline.ApplicationIntentCandidateSchemaV1,
					ProductContext: "A browser counter",
					Requirements: []string{
						"Show the current count.",
						"Provide controls that increment and reset the count.",
					},
				}
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected semantic work kind %q", job.Kind)
			}
			raw, marshalErr := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, marshalErr
		},
	}
	specification, err := runDirectCodingApplicationInterpreter(
		runtime, "intent-model", "surface-model", "artifact-model",
		request, applicationContext, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if counts[assemblyline.WorkApplicationContextNeeds] != 0 ||
		counts[assemblyline.WorkApplicationIntent] != 1 ||
		counts[assemblyline.WorkApplicationClassify] != 1 {
		t.Fatalf("front-door calls=%v", counts)
	}
	want := []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "Show the current count."},
		{ID: "requirement_002", SourceQuote: "Provide controls that increment and reset the count."},
	}
	if specification.ProductQuote != "A browser counter" ||
		!reflect.DeepEqual(specification.Requirements, want) {
		t.Fatalf("specification=%+v", specification)
	}
}

func TestApplicationFrontDoorFailsLoudlyWhenEvidenceNeedsAreUnresolved(t *testing.T) {
	t.Parallel()
	const request = "Extend the existing application with the established reporting behavior."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceExisting, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(kind string, _ string, _ string, _ map[string]any) (string, error) {
			if kind != string(assemblyline.WorkApplicationContextNeeds) {
				return "", fmt.Errorf("unexpected semantic work kind %q", kind)
			}
			return `{"schema":"omnidex.application-context-needs.v1","questions":["What verified behavior is meant by the established reporting behavior?"]}`, nil
		}),
	}
	_, err = runDirectCodingApplicationInterpreter(
		runtime, "intent-model", "surface-model", "artifact-model",
		request, applicationContext, nil,
	)
	if err == nil {
		t.Fatal("unresolved evidence need silently continued")
	}
}
