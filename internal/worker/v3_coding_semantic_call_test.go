package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
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

func TestSemanticCallRejectsKnownBareArtifactBeforeInference(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{"internal/transport.go"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "Update transport.go."},
	)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
		Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
			called = true
			return assemblyline.PortableResult{}, nil
		},
	}
	_, err = runDirectCodingSemanticCall[assemblyline.ApplicationClassification](
		runtime, "semantic", "classification", job, nil,
		func(value assemblyline.ApplicationClassification) error { return value.Validate() },
	)
	if err == nil || called || !strings.Contains(err.Error(), "known artifact identity") {
		t.Fatalf("known artifact prompt err=%v inference_called=%t", err, called)
	}
}

func TestSemanticCallRejectsKnownBareArtifactAtCandidateAcceptance(t *testing.T) {
	t.Parallel()
	const request = "Build a small service."
	contextAuthority, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationIntentJob(assemblyline.ApplicationIntentInput{
		UserRequest: request, Context: contextAuthority,
	})
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{"internal/transport.go"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, PathProvenance: provenance,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{
				JobID:     job.ID,
				Candidate: `{"schema":"omnidex.application-intent.v1","product_context":"transport.go service","requirements":["Return one value"]}`,
			}, nil
		},
	}
	_, err = runDirectCodingSemanticCall[assemblyline.ApplicationIntentCandidate](
		runtime, "semantic", "application_intent", job, nil,
		func(value assemblyline.ApplicationIntentCandidate) error { return value.Validate() },
	)
	if err == nil || !strings.Contains(err.Error(), "filesystem identity") || calls != 1 {
		t.Fatalf("known artifact candidate error=%v calls=%d", err, calls)
	}
}

func TestSemanticCallNeverRetainsQualifiedPathCandidate(t *testing.T) {
	t.Parallel()
	const request = "Build a small service."
	contextAuthority, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationIntentJob(assemblyline.ApplicationIntentInput{
		UserRequest: request, Context: contextAuthority,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, productContext := range map[string]string{
		"POSIX absolute":     "/etc/passwd service",
		"relative traversal": "../private/value service",
		"tilde":              "~/private/value service",
		"UNC":                `\\server\share\value service`,
		"Windows absolute":   `C:\private\value service`,
	} {
		name, productContext := name, productContext
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, marshalErr := json.Marshal(assemblyline.ApplicationIntentCandidate{
				Schema:         assemblyline.ApplicationIntentCandidateSchemaV1,
				ProductContext: productContext,
				Requirements:   []string{"Return one value"},
			})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 3,
				Execute: func(portable assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					return assemblyline.PortableResult{JobID: portable.ID, Candidate: string(raw)}, nil
				},
			}
			_, callErr := runDirectCodingSemanticCall[assemblyline.ApplicationIntentCandidate](
				runtime, "semantic", "application_intent", job, nil,
				func(value assemblyline.ApplicationIntentCandidate) error { return value.Validate() },
			)
			var exhausted *semanticCandidateExhaustedError
			if callErr == nil || !strings.Contains(callErr.Error(), "filesystem identity") ||
				calls != 1 || errors.As(callErr, &exhausted) {
				t.Fatalf("path candidate error=%v calls=%d recoverable=%t", callErr, calls, exhausted != nil)
			}
		})
	}
}

func TestSemanticCallRetainsUnprovenDottedProductName(t *testing.T) {
	t.Parallel()
	const request = "Build a small service."
	contextAuthority, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationIntentJob(assemblyline.ApplicationIntentInput{
		UserRequest: request, Context: contextAuthority,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			return `{"schema":"omnidex.application-intent.v1","product_context":"Node.js service with Vue.js interface","requirements":["Return one value"]}`, nil
		}),
	}
	result, err := runDirectCodingSemanticCall[assemblyline.ApplicationIntentCandidate](
		runtime, "semantic", "application_intent", job, nil,
		func(value assemblyline.ApplicationIntentCandidate) error { return value.Validate() },
	)
	if err != nil || !strings.Contains(result.ProductContext, "Node.js") {
		t.Fatalf("dotted product candidate=%+v err=%v", result, err)
	}
}

func TestSemanticCallRejectsKnownBareArtifactInRepairGuidance(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewTypeScriptRepairGuidanceJob(
		assemblyline.TypeScriptRepairGuidanceInput{
			Language: "typescript", Dialect: "TypeScript function syntax",
			Signature:          "function value(): number",
			CurrentDeclaration: "function value(): number { return 1; }",
			Diagnostic:         "error TS2322: Type string is not assignable to type number.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance([]string{"internal/transport.go"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, PathProvenance: provenance,
		Execute: testPortableExecutor(func(_ string, _ string, _ string, _ map[string]any) (string, error) {
			return `{"instruction":"Move the value into transport.go."}`, nil
		}),
	}
	_, err = runDirectCodingSemanticCall[assemblyline.TypeScriptRepairGuidance](
		runtime, "semantic", "repair_guidance", job, nil,
		func(value assemblyline.TypeScriptRepairGuidance) error { return value.Validate() },
	)
	if err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("known artifact guidance error=%v", err)
	}
}

func TestDirectCodingSemanticCorrectionReceivesExactQuestionCandidateAndDefect(t *testing.T) {
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
	if len(prompts) != 2 || !strings.Contains(prompts[1], rejected) ||
		!strings.Contains(prompts[1], "Build a browser tool.") ||
		!strings.Contains(prompts[1], "explicitly asks for a browser tool") {
		t.Fatalf("semantic correction omitted exact retained authority or defect:\n%s", strings.Join(prompts, "\n---\n"))
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

func TestDecodeCodingSemanticJSONRejectsInexactObjects(t *testing.T) {
	t.Parallel()

	type response struct {
		Value string `json:"value"`
	}
	for _, raw := range []string{
		`{"value":"ok","unknown":true}`,
		`{"value":"first","value":"second"}`,
		`{"Value":"alias"}`,
		`{"value":"ok"} {}`,
		"```json\n{\"value\":\"ok\"}\n```",
	} {
		if _, err := decodeDirectCodingSemanticJSON[response](raw); err == nil {
			t.Fatalf("accepted malformed semantic response %q", raw)
		}
	}
}
