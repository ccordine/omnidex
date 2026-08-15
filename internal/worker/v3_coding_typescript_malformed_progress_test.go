package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptCorrectionRetainsCurrentDeclarationAcrossMalformedResponse(t *testing.T) {
	fixtures := []struct {
		name      string
		tsx       bool
		signature string
		current   string
		fixed     string
	}{
		{
			name:      "numeric calculation",
			signature: "function apply(value: number): number",
			current:   "function apply(value: number): number { return missingValue; }",
			fixed:     "function apply(value: number): number { return value + 1; }",
		},
		{
			name:      "status view",
			tsx:       true,
			signature: "function renderStatus(label: string): ReactElement",
			current:   "function renderStatus(label: string): ReactElement { return <p>{missingLabel}</p>; }",
			fixed:     "function renderStatus(label: string): ReactElement { return <p>{label}</p>; }",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			calls := 0
			const malformed = "I need more context before I can return source."
			job := directCodingTypeScriptFragmentJob{
				block: assemblyline.TypeScriptBlock{
					ID: "feature.repair", Signature: fixture.signature, API: fixture.signature,
				},
				tsx: fixture.tsx, current: fixture.current,
				failure: "error TS2304: Cannot find name.",
			}
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 1, CorrectionModel: "corrector",
				Execute: func(portable assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					calls++
					candidate := malformed
					if calls == 2 {
						var correction assemblyline.FragmentCorrectionInput
						if err := json.Unmarshal(portable.Payload, &correction); err != nil {
							t.Fatal(err)
						}
						if correction.CurrentDeclaration != fixture.current {
							t.Fatalf("current declaration was not retained:\n%s", correction.CurrentDeclaration)
						}
						if strings.Contains(correction.Diagnostic, malformed) ||
							!strings.Contains(correction.Diagnostic, "CORRECTION_REJECTION:") {
							t.Fatalf("correction diagnostic crossed the raw-response boundary: %q", correction.Diagnostic)
						}
						candidate = fixture.fixed
					}
					return assemblyline.PortableResult{JobID: portable.ID, Candidate: candidate}, nil
				},
			}

			got, err := runDirectCodingTypeScriptFragmentWorker(runtime, "corrector", job)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || got != fixture.fixed {
				t.Fatalf("calls=%d source=%q", calls, got)
			}
		})
	}
}

func TestTypeScriptCorrectionStopsRepeatedMalformedResponseCycle(t *testing.T) {
	const current = "function apply(value: number): number { return missingValue; }"
	calls := 0
	job := directCodingTypeScriptFragmentJob{
		block: assemblyline.TypeScriptBlock{
			ID: "feature.repair", Signature: "function apply(value: number): number",
			API: "function apply(value: number): number",
		},
		current: current, failure: "error TS2304: Cannot find name 'missingValue'.",
	}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1, CorrectionModel: "corrector",
		Execute: func(portable assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{JobID: portable.ID, Candidate: "No function is available."}, nil
		},
	}

	_, err := runDirectCodingTypeScriptFragmentWorker(runtime, "corrector", job)
	if err == nil || !strings.Contains(err.Error(), "repeated candidate/diagnostic correction state") {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
	if calls != 2 {
		t.Fatalf("repeated malformed response dispatched %d calls, want 2", calls)
	}
}
