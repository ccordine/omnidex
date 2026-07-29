package worker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingSemanticCallReportsPromptMeasurementWithoutContents(t *testing.T) {
	t.Parallel()

	input := assemblyline.ApplicationClassificationInput{UserRequest: "Build a small browser tool."}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	var started typedWorkerEvent
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			return `{"schema":"omnidex.application-class.v1","surface":"browser_application"}`, nil
		}),
		Emit: func(event typedWorkerEvent) {
			if event.State == typedWorkerStarted {
				started = event
			}
		},
	}
	result, err := runDirectCodingSemanticCall[assemblyline.ApplicationClassification](
		runtime, "semantic", "classification", job, nil,
		func(value assemblyline.ApplicationClassification) error { return value.Validate() },
	)
	if err != nil || result.Surface != assemblyline.ApplicationSurfaceBrowser {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if started.PromptBytes != len(prompt) {
		t.Fatalf("semantic start did not report prompt size: %#v", started)
	}
	if rendered := renderDirectCodingWorkerEvent(started); strings.Contains(rendered, prompt) {
		t.Fatalf("semantic status exposed prompt contents: %s", rendered)
	}
}

func TestDirectCodingSemanticCorrectionPatchesRetainedCandidateWithoutReplayingIt(t *testing.T) {
	t.Parallel()

	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "Build a browser tool."},
	)
	if err != nil {
		t.Fatal(err)
	}
	const rejected = `{"schema":"omnidex.application-class.v1","surface":"unsupported"}`
	var prompts []string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: testPortableExecutor(func(_ string, _ string, prompt string, _ map[string]any) (string, error) {
			prompts = append(prompts, prompt)
			if len(prompts) == 1 {
				return rejected, nil
			}
			return `{"surface":"browser_application"}`, nil
		}),
	}
	_, err = runDirectCodingSemanticCall[assemblyline.ApplicationClassification](
		runtime, "semantic", "classification", job, nil,
		func(value assemblyline.ApplicationClassification) error {
			if value.Surface == assemblyline.ApplicationSurfaceUnsupported {
				return errors.New("the request explicitly asks for a browser tool")
			}
			return value.Validate()
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 || strings.Contains(prompts[1], rejected) ||
		strings.Contains(prompts[1], "Build a browser tool.") ||
		!strings.Contains(prompts[1], "explicitly asks for a browser tool") {
		t.Fatalf("semantic correction replayed retained context or omitted direct failure:\n%s", strings.Join(prompts, "\n---\n"))
	}
}

func TestSemanticCallTypesOnlyCandidateExhaustionAsRecoverable(t *testing.T) {
	t.Parallel()

	input := assemblyline.ApplicationClassificationInput{UserRequest: "Build a browser tool."}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		execute     func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error)
		recoverable bool
	}{
		{
			name: "invalid candidate",
			execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
				return `{"schema":"omnidex.application-class.v1","surface":"invalid"}`, nil
			}),
			recoverable: true,
		},
		{
			name: "execution failure",
			execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
				return assemblyline.PortableResult{}, errors.New("model endpoint unavailable")
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, Execute: test.execute,
			}
			_, callErr := runDirectCodingSemanticCall[assemblyline.ApplicationClassification](
				runtime, "semantic", "classification", job, nil,
				func(value assemblyline.ApplicationClassification) error { return value.Validate() },
			)
			var exhausted *semanticCandidateExhaustedError
			if got := errors.As(callErr, &exhausted); got != test.recoverable {
				t.Fatalf("recoverable=%v error=%v", got, callErr)
			}
		})
	}
}

func TestDecodeCodingSemanticJSONRejectsUnknownAndTrailingData(t *testing.T) {
	t.Parallel()

	type response struct {
		Value string `json:"value"`
	}
	for _, raw := range []string{`{"value":"ok","unknown":true}`, `{"value":"ok"} {}`} {
		if _, err := decodeDirectCodingSemanticJSON[response](raw); err == nil {
			t.Fatalf("accepted malformed semantic response %q", raw)
		}
	}
}
