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
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[assemblyline.WorkKind]int)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(job assemblyline.PortableJob, modelName string) (assemblyline.PortableResult, error) {
			_, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			counts[job.Kind]++
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationClassify:
				candidate = string(assemblyline.ApplicationSurfaceBrowser)
			case assemblyline.WorkApplicationProductContext:
				candidate = "A browser counter"
			case assemblyline.WorkApplicationRequirementCoverage:
				var input assemblyline.ApplicationRequirementLeafInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.AcceptedRequirements == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement coverage received a nil accepted set")
				}
				if len(input.AcceptedRequirements) == 0 {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement coverage received an empty accepted set")
				}
				if len(input.AcceptedRequirements) < 2 {
					candidate = assemblyline.ApplicationRequirementRemains
				} else {
					candidate = assemblyline.ApplicationNoUncoveredRequirement
				}
			case assemblyline.WorkApplicationRequirement:
				var input assemblyline.ApplicationRequirementLeafInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.AcceptedRequirements == nil {
					return assemblyline.PortableResult{}, fmt.Errorf("application requirement received a nil accepted set")
				}
				candidate = []string{
					"Show the current count.",
					"Provide controls that increment and reset the count.",
				}[counts[job.Kind]-1]
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected semantic work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	specification, err := runDirectCodingApplicationInterpreter(
		runtime, "intent-model", "surface-model", "artifact-model",
		request, applicationContext, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if counts[assemblyline.WorkApplicationContextNeedCoverage] != 0 ||
		counts[assemblyline.WorkApplicationProductContext] != 1 ||
		counts[assemblyline.WorkApplicationRequirementCoverage] != 2 ||
		counts[assemblyline.WorkApplicationRequirement] != 2 ||
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
		request, assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}
	coverageCalls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(kind string, _ string, _ string) (string, error) {
			switch assemblyline.WorkKind(kind) {
			case assemblyline.WorkApplicationContextNeedCoverage:
				coverageCalls++
				if coverageCalls == 1 {
					return assemblyline.ApplicationContextNeedRemains, nil
				}
				return assemblyline.ApplicationNoUncoveredContextNeed, nil
			case assemblyline.WorkApplicationContextNeedQuestion:
				return "What verified behavior is meant by the established reporting behavior?", nil
			default:
				return "", fmt.Errorf("unexpected semantic work kind %q", kind)
			}
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
