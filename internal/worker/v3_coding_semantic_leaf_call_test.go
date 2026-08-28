package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingSemanticLeafCallBindsRawResultWithStationDecoder(t *testing.T) {
	t.Parallel()
	input := assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(current assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			calls++
			if current.ID != job.ID || model != "semantic-model" {
				t.Fatalf("current=%+v model=%q", current, model)
			}
			return assemblyline.PortableResult{
				JobID: current.ID, Candidate: "browser_application",
			}, nil
		},
	}
	result, err := runDirectCodingSemanticLeafCall(
		runtime, "semantic-model", "surface", job, nil,
		func(raw string) (assemblyline.ApplicationClassification, error) {
			return assemblyline.DecodeApplicationClassification(input, raw)
		},
		func(value assemblyline.ApplicationClassification) error {
			return value.Validate()
		},
	)
	if err != nil || result.Surface != assemblyline.ApplicationSurfaceBrowser || calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
	}
}

func TestDirectCodingSemanticLeafCorrectionReturnsCompleteRawReplacement(t *testing.T) {
	t.Parallel()
	input := assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []assemblyline.WorkKind
	var correctionPrompt string
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: func(current assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			kinds = append(kinds, current.Kind)
			candidate := `{"surface":"browser_application"}`
			if current.Kind == assemblyline.WorkResponseCorrection {
				var correction assemblyline.ResponseCorrectionInput
				if err := json.Unmarshal(current.Payload, &correction); err != nil {
					t.Fatal(err)
				}
				if correction.Original.ID != job.ID ||
					correction.RetainedCandidate != `{"surface":"browser_application"}` {
					t.Fatalf("correction authority=%+v", correction)
				}
				correctionPrompt, err = assemblyline.RenderPortableJob(current)
				if err != nil {
					t.Fatal(err)
				}
				candidate = "browser_application"
			}
			return assemblyline.PortableResult{JobID: current.ID, Candidate: candidate}, nil
		},
	}
	result, err := runDirectCodingSemanticLeafCall(
		runtime, "semantic-model", "surface", job, nil,
		func(raw string) (assemblyline.ApplicationClassification, error) {
			return assemblyline.DecodeApplicationClassification(input, raw)
		},
		func(value assemblyline.ApplicationClassification) error {
			return value.Validate()
		},
	)
	if err != nil || result.Surface != assemblyline.ApplicationSurfaceBrowser {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(kinds) != 2 || kinds[0] != assemblyline.WorkApplicationClassify ||
		kinds[1] != assemblyline.WorkResponseCorrection {
		t.Fatalf("work kinds=%v", kinds)
	}
	for _, required := range []string{
		"complete replacement leaf", "CURRENT_REJECTED_LEAF",
		`{"surface":"browser_application"}`,
	} {
		if !strings.Contains(correctionPrompt, required) {
			t.Fatalf("correction prompt omitted %q:\n%s", required, correctionPrompt)
		}
	}
	if strings.Contains(correctionPrompt, "merge patch") {
		t.Fatalf("correction prompt retains merge-patch authority:\n%s", correctionPrompt)
	}
}

func TestDirectCodingSemanticLeafStopsOnZeroDeltaCorrection(t *testing.T) {
	t.Parallel()
	input := assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(current assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{
				JobID: current.ID, Candidate: `{"surface":"browser_application"}`,
			}, nil
		},
	}
	_, err = runDirectCodingSemanticLeafCall(
		runtime, "semantic-model", "surface", job, nil,
		func(raw string) (assemblyline.ApplicationClassification, error) {
			return assemblyline.DecodeApplicationClassification(input, raw)
		},
		func(value assemblyline.ApplicationClassification) error {
			return value.Validate()
		},
	)
	if err == nil || calls != 2 {
		t.Fatalf("zero-delta correction calls=%d err=%v", calls, err)
	}
}

func TestDirectCodingSemanticLeafNeverNormalizesModelBytes(t *testing.T) {
	t.Parallel()
	input := assemblyline.ApplicationClassificationInput{
		UserRequest: "Build a small browser tool.",
	}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		name       string
		candidates []string
	}{
		{name: "initial leaf", candidates: []string{" browser_application "}},
		{name: "replacement leaf", candidates: []string{
			`{"surface":"browser_application"}`, " browser_application ",
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := 0
			runtime := typedWorkerRuntime{
				Context: context.Background(), MaxAttempts: 3,
				Execute: func(current assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
					candidate := testCase.candidates[calls]
					calls++
					return assemblyline.PortableResult{JobID: current.ID, Candidate: candidate}, nil
				},
			}
			_, err := runDirectCodingSemanticLeafCall(
				runtime, "semantic-model", "surface", job, nil,
				func(raw string) (assemblyline.ApplicationClassification, error) {
					return assemblyline.DecodeApplicationClassification(input, raw)
				},
				func(value assemblyline.ApplicationClassification) error {
					return value.Validate()
				},
			)
			if err == nil || calls != len(testCase.candidates) {
				t.Fatalf("model bytes were normalized: calls=%d error=%v", calls, err)
			}
		})
	}
}
